package usecases

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"payment-kita.backend/internal/domain/entities"
	domainerrors "payment-kita.backend/internal/domain/errors"
	domainrepos "payment-kita.backend/internal/domain/repositories"
	"payment-kita.backend/pkg/utils"
)

type CreatePaymentPricingType string

const (
	CreatePaymentPricingTypeInvoiceCurrency   CreatePaymentPricingType = "invoice_currency"
	CreatePaymentPricingTypePaymentTokenFixed CreatePaymentPricingType = "payment_token_fixed"
	CreatePaymentPricingTypePaymentTokenDyn   CreatePaymentPricingType = "payment_token_dynamic"

	defaultCreatePaymentQuoteStageTimeout   = 20 * time.Second
	defaultCreatePaymentSessionStageTimeout = 8 * time.Second
	defaultCreatePaymentTTL                 = 3 * time.Minute
	minCreatePaymentTTL                     = 30 * time.Second
	maxCreatePaymentTTL                     = 24 * time.Hour
	createPaymentUnlimitedValue             = "unlimited"
	maxCreatePaymentUpperBoundDoublings     = 32
	maxCreatePaymentBinarySearchIterations  = 64
)

var createPaymentUnlimitedExpiryAt = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)

type CreatePaymentInput struct {
	MerchantContextID uuid.UUID
	MerchantID        uuid.UUID
	ChainID           string
	SelectedToken     string
	PricingType       string
	RequestedAmount   string
	ExpiresIn         string
}

type CreatePaymentOutput struct {
	PaymentID                string    `json:"payment_id"`
	MerchantID               string    `json:"merchant_id"`
	Amount                   string    `json:"amount"`
	InvoiceCurrency          string    `json:"invoice_currency"`
	InvoiceAmount            string    `json:"invoice_amount"`
	SettlementAmount         string    `json:"settlement_amount"`
	SettlementAmountAtomic   string    `json:"settlement_amount_atomic"`
	PayerSelectedChain       string    `json:"payer_selected_chain"`
	PayerSelectedToken       string    `json:"payer_selected_token"`
	PayerSelectedTokenSymbol string    `json:"payer_selected_token_symbol"`
	QuotedTokenSymbol        string    `json:"quoted_token_symbol"`
	QuotedTokenAmount        string    `json:"quoted_token_amount"`
	QuotedTokenAmountAtomic  string    `json:"quoted_token_amount_atomic"`
	QuotedTokenDecimals      int       `json:"quoted_token_decimals"`
	QuoteRate                string    `json:"quote_rate"`
	QuoteSource              string    `json:"quote_source"`
	QuoteExpiresAt           time.Time `json:"quote_expires_at"`
	IsUnlimitedExpiry        bool      `json:"is_unlimited_expiry"`
	DestChain                string    `json:"dest_chain"`
	DestToken                string    `json:"dest_token"`
	DestWallet               string    `json:"dest_wallet"`
	SettlementDestChain      string    `json:"settlement_dest_chain"`
	SettlementDestToken      string    `json:"settlement_dest_token"`
	SettlementDestWallet     string    `json:"settlement_dest_wallet"`
	ExpireTime               time.Time `json:"expire_time"`
	PaymentURL               string    `json:"payment_url"`
	PaymentCode              string    `json:"payment_code"`
	FeeBreakdown             struct {
		PlatformFee        string `json:"platform_fee"`
		BridgeFee          string `json:"bridge_fee"`
		TotalFee           string `json:"total_fee"`
		TotalFeePercentage string `json:"total_fee_percentage"`
	} `json:"fee_breakdown"`
	RouteInfo struct {
		Path        []string `json:"path"`
		HopCount    int      `json:"hop_count"`
		IsDirect    bool     `json:"is_direct"`
		RouteSource string   `json:"route_source"`
	} `json:"route_info"`
	PaymentInstruction struct {
		ChainID     string `json:"chain_id"`
		To          string `json:"to,omitempty"`
		Value       string `json:"value,omitempty"`
		Data        string `json:"data,omitempty"`
		ProgramID   string `json:"program_id,omitempty"`
		DataBase58  string `json:"data_base58,omitempty"`
		DataBase64  string `json:"data_base64,omitempty"`
		ApprovalTo  string `json:"approval_to,omitempty"`
		ApprovalHex string `json:"approval_hex_data,omitempty"`
	} `json:"payment_instruction"`
}

type createPaymentQuoteEngine interface {
	CreateQuote(ctx context.Context, input *CreatePartnerQuoteInput) (*CreatePartnerQuoteOutput, error)
}

type createPaymentQuotePreviewEngine interface {
	PreviewQuote(ctx context.Context, input *CreatePartnerQuoteInput) (*CreatePartnerQuoteOutput, error)
}

type createPaymentQuoteInversePreviewEngine interface {
	PreviewRequiredInputForOutput(ctx context.Context, input *PreviewRequiredInputForOutputInput) (*PreviewRequiredInputForOutputOutput, error)
}

type createPaymentSessionEngine interface {
	CreateSession(ctx context.Context, input *CreatePartnerPaymentSessionInput) (*CreatePartnerPaymentSessionOutput, error)
}

type CreatePaymentUsecase struct {
	merchantRepo   domainrepos.MerchantRepository
	settlementRepo domainrepos.MerchantSettlementProfileRepository
	walletRepo     domainrepos.WalletRepository
	tokenRepo      domainrepos.TokenRepository
	chainRepo      domainrepos.ChainRepository
	quoteRepo      domainrepos.PaymentQuoteRepository
	sessionRepo    domainrepos.PartnerPaymentSessionRepository
	quoteUC        createPaymentQuoteEngine
	sessionUC      createPaymentSessionEngine
	chainResolver  *ChainResolver
}

type merchantCreatePaymentConfig struct {
	DestChain         string
	DestToken         string
	DestWallet        string
	BridgeTokenSymbol string
}

type resolvedMerchantSettlementConfig struct {
	InvoiceToken      *entities.Token
	DestChainID       uuid.UUID
	DestChainCAIP2    string
	DestToken         *entities.Token
	DestWallet        string
	BridgeTokenSymbol string
	DestBridgeToken   *entities.Token
}

func NewCreatePaymentUsecase(
	merchantRepo domainrepos.MerchantRepository,
	settlementRepo domainrepos.MerchantSettlementProfileRepository,
	walletRepo domainrepos.WalletRepository,
	tokenRepo domainrepos.TokenRepository,
	chainRepo domainrepos.ChainRepository,
	quoteRepo domainrepos.PaymentQuoteRepository,
	sessionRepo domainrepos.PartnerPaymentSessionRepository,
	quoteUC createPaymentQuoteEngine,
	sessionUC createPaymentSessionEngine,
) *CreatePaymentUsecase {
	return &CreatePaymentUsecase{
		merchantRepo:   merchantRepo,
		settlementRepo: settlementRepo,
		walletRepo:     walletRepo,
		tokenRepo:      tokenRepo,
		chainRepo:      chainRepo,
		quoteRepo:      quoteRepo,
		sessionRepo:    sessionRepo,
		quoteUC:        quoteUC,
		sessionUC:      sessionUC,
		chainResolver:  NewChainResolver(chainRepo),
	}
}

func (u *CreatePaymentUsecase) CreatePayment(ctx context.Context, input *CreatePaymentInput) (*CreatePaymentOutput, error) {
	startedAt := time.Now()
	ctx = withQuoteRequestCache(ctx)
	ctx = withPreferDryRunQuote(ctx)
	if input != nil {
		createPaymentTraceInfo(ctx, "create_payment.start",
			zap.String("merchant_context_id", input.MerchantContextID.String()),
			zap.String("merchant_id", input.MerchantID.String()),
			zap.String("chain_id", strings.TrimSpace(input.ChainID)),
			zap.String("selected_token", strings.TrimSpace(input.SelectedToken)),
			zap.String("pricing_type", strings.TrimSpace(input.PricingType)),
			zap.String("requested_amount", strings.TrimSpace(input.RequestedAmount)),
			zap.String("expires_in", strings.TrimSpace(input.ExpiresIn)),
			zap.Bool("prefer_dry_run_quote", preferDryRunQuote(ctx)),
		)
	}
	if input == nil {
		return nil, domainerrors.BadRequest("input is required")
	}
	merchantID, err := u.resolveMerchantID(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ChainID) == "" {
		return nil, domainerrors.BadRequest("chain_id is required")
	}
	if strings.TrimSpace(input.SelectedToken) == "" {
		return nil, domainerrors.BadRequest("selected_token is required")
	}
	if strings.TrimSpace(input.RequestedAmount) == "" {
		return nil, domainerrors.BadRequest("requested_amount is required")
	}
	pricingType := CreatePaymentPricingType(strings.TrimSpace(input.PricingType))
	if pricingType == "" {
		return nil, domainerrors.BadRequest("pricing_type is required")
	}
	if pricingType != CreatePaymentPricingTypeInvoiceCurrency && pricingType != CreatePaymentPricingTypePaymentTokenFixed && pricingType != CreatePaymentPricingTypePaymentTokenDyn {
		return nil, domainerrors.BadRequest("unsupported pricing_type")
	}
	if u.quoteUC == nil || u.sessionUC == nil || u.quoteRepo == nil || u.merchantRepo == nil || u.walletRepo == nil || u.tokenRepo == nil || u.chainRepo == nil || u.settlementRepo == nil {
		return nil, domainerrors.InternalServerError("create payment orchestrator is not configured")
	}
	createPaymentTraceDebug(ctx, "create_payment.validated_input",
		zap.String("merchant_id", merchantID.String()),
		zap.String("pricing_type", string(pricingType)),
	)

	merchant, err := u.merchantRepo.GetByID(ctx, merchantID)
	if err != nil || merchant == nil {
		return nil, domainerrors.NotFound("merchant not found")
	}
	if merchant.Status != entities.MerchantStatusActive {
		return nil, domainerrors.BadRequest("merchant account is not active")
	}

	config := u.resolveMerchantCreatePaymentConfig(ctx, merchant)
	settlement, err := u.resolveMerchantSettlementConfig(ctx, config)
	if err != nil {
		createPaymentTraceWarn(ctx, "create_payment.settlement_config_failed",
			zap.String("merchant_id", merchantID.String()),
			zap.Error(err),
		)
		return nil, err
	}
	chainUUID, chainCAIP2, err := u.chainResolver.ResolveFromAny(ctx, input.ChainID)
	if err != nil {
		return nil, domainerrors.BadRequest(fmt.Sprintf("invalid chain_id: %v", err))
	}
	selectedToken, err := u.tokenRepo.GetByAddress(ctx, strings.TrimSpace(input.SelectedToken), chainUUID)
	if err != nil || selectedToken == nil || !selectedToken.IsActive {
		return nil, domainerrors.BadRequest("selected_token not supported on chain_id")
	}
	walletAddress, err := u.resolveMerchantWallet(ctx, merchant, config)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt, isUnlimitedExpiry, err := resolveCreatePaymentExpiresAt(strings.TrimSpace(input.ExpiresIn), now)
	if err != nil {
		return nil, err
	}
	createPaymentTraceInfo(ctx, "create_payment.context_resolved",
		zap.String("merchant_id", merchantID.String()),
		zap.String("selected_chain_caip2", chainCAIP2),
		zap.String("selected_token_symbol", strings.TrimSpace(selectedToken.Symbol)),
		zap.String("selected_token_address", strings.TrimSpace(selectedToken.ContractAddress)),
		zap.String("settlement_chain_caip2", settlement.DestChainCAIP2),
		zap.String("settlement_token_symbol", strings.TrimSpace(settlement.DestToken.Symbol)),
		zap.String("settlement_token_address", strings.TrimSpace(settlement.DestToken.ContractAddress)),
		zap.String("bridge_token_symbol", strings.TrimSpace(settlement.BridgeTokenSymbol)),
		zap.String("merchant_dest_wallet", walletAddress),
		zap.String("expires_at", expiresAt.Format(time.RFC3339)),
		zap.Bool("is_unlimited_expiry", isUnlimitedExpiry),
	)

	var quoteOut *CreatePartnerQuoteOutput
	var invoiceAtomic string
	quoteStartedAt := time.Now()
	quoteStageTimeout := createPaymentStageTimeoutFromEnv("CREATE_PAYMENT_QUOTE_STAGE_TIMEOUT_MS", defaultCreatePaymentQuoteStageTimeout)
	quoteCtx, quoteCancel := context.WithTimeout(ctx, createPaymentStageTimeoutFromEnv("CREATE_PAYMENT_QUOTE_STAGE_TIMEOUT_MS", defaultCreatePaymentQuoteStageTimeout))
	defer quoteCancel()
	createPaymentTraceInfo(ctx, "create_payment.quote_stage_start",
		zap.String("pricing_type", string(pricingType)),
		zap.Duration("timeout", quoteStageTimeout),
	)
	switch pricingType {
	case CreatePaymentPricingTypeInvoiceCurrency:
		quoteOut, err = u.createInvoiceCurrencyQuote(quoteCtx, merchantID, chainCAIP2, selectedToken, settlement, strings.TrimSpace(input.RequestedAmount), walletAddress, expiresAt)
		// Capture invoiceAtomic for response
		invoiceAtomic, _ = convertToSmallestUnit(strings.TrimSpace(input.RequestedAmount), settlement.InvoiceToken.Decimals)
	default:
		quoteOut, err = u.createSyntheticSelectedTokenQuote(quoteCtx, merchantID, chainCAIP2, selectedToken, pricingType, strings.TrimSpace(input.RequestedAmount), settlement, expiresAt)
		invoiceAtomic = strings.TrimSpace(input.RequestedAmount)
	}
	if err != nil {
		createPaymentTraceWarn(ctx, "create_payment.quote_stage_failed",
			zap.String("pricing_type", string(pricingType)),
			zap.Duration("latency", time.Since(quoteStartedAt)),
			zap.Error(err),
		)
		return nil, err
	}
	createPaymentTraceInfo(ctx, "create_payment.quote_stage_success",
		zap.Duration("latency", time.Since(quoteStartedAt)),
		zap.String("quote_id", strings.TrimSpace(quoteOut.QuoteID)),
		zap.String("quoted_amount_atomic", strings.TrimSpace(quoteOut.QuotedAmount)),
		zap.Int("quoted_decimals", quoteOut.QuoteDecimals),
		zap.String("route", strings.TrimSpace(quoteOut.Route)),
		zap.String("price_source", strings.TrimSpace(quoteOut.PriceSource)),
		zap.String("quote_expires_at", quoteOut.QuoteExpiresAt.UTC().Format(time.RFC3339)),
	)

	quoteID, err := uuid.Parse(quoteOut.QuoteID)
	if err != nil {
		return nil, domainerrors.InternalServerError("invalid quote id generated")
	}
	sessionStartedAt := time.Now()
	sessionStageTimeout := createPaymentStageTimeoutFromEnv("CREATE_PAYMENT_SESSION_STAGE_TIMEOUT_MS", defaultCreatePaymentSessionStageTimeout)
	sessionCtx, sessionCancel := context.WithTimeout(ctx, sessionStageTimeout)
	defer sessionCancel()
	createPaymentTraceInfo(ctx, "create_payment.session_stage_start",
		zap.String("quote_id", quoteID.String()),
		zap.Duration("timeout", sessionStageTimeout),
	)
	sessionOut, err := u.sessionUC.CreateSession(sessionCtx, &CreatePartnerPaymentSessionInput{
		MerchantID:        merchantID,
		QuoteID:           quoteID,
		DestWallet:        walletAddress,
		DestChainOverride: settlement.DestChainCAIP2,
		DestTokenOverride: settlement.DestToken.ContractAddress,
	})
	if err != nil {
		createPaymentTraceWarn(ctx, "create_payment.session_stage_failed",
			zap.String("quote_id", quoteID.String()),
			zap.Duration("latency", time.Since(sessionStartedAt)),
			zap.Error(err),
		)
		return nil, err
	}
	createPaymentTraceInfo(ctx, "create_payment.session_stage_success",
		zap.Duration("latency", time.Since(sessionStartedAt)),
		zap.String("payment_id", strings.TrimSpace(sessionOut.PaymentID)),
		zap.String("session_dest_chain", strings.TrimSpace(sessionOut.DestChain)),
		zap.String("session_dest_token", strings.TrimSpace(sessionOut.DestToken)),
		zap.String("instruction_chain", strings.TrimSpace(sessionOut.PaymentInstruction.ChainID)),
		zap.String("instruction_to", strings.TrimSpace(sessionOut.PaymentInstruction.To)),
		zap.String("instruction_value", strings.TrimSpace(sessionOut.PaymentInstruction.Value)),
	)

	out := &CreatePaymentOutput{
		PaymentID:                sessionOut.PaymentID,
		MerchantID:               sessionOut.MerchantID,
		Amount:                   sessionOut.Amount,
		InvoiceCurrency:          settlement.InvoiceToken.Symbol,
		InvoiceAmount:            strings.TrimSpace(input.RequestedAmount),
		SettlementAmount:         strings.TrimSpace(input.RequestedAmount), // Net amount merchant receives (same as invoice for now)
		SettlementAmountAtomic:   invoiceAtomic,                            // Atomic representation
		PayerSelectedChain:       chainCAIP2,
		PayerSelectedToken:       selectedToken.ContractAddress,
		PayerSelectedTokenSymbol: selectedToken.Symbol,
		QuotedTokenSymbol:        quoteOut.SelectedTokenSymbol,
		QuotedTokenAmount:        smallestUnitToDecimalString(quoteOut.QuotedAmount, quoteOut.QuoteDecimals),
		QuotedTokenAmountAtomic:  quoteOut.QuotedAmount,
		QuotedTokenDecimals:      quoteOut.QuoteDecimals,
		QuoteRate:                quoteOut.QuoteRate,
		QuoteSource:              quoteOut.PriceSource,
		QuoteExpiresAt:           quoteOut.QuoteExpiresAt,
		IsUnlimitedExpiry:        isUnlimitedExpiry,
		DestChain:                sessionOut.DestChain,
		DestToken:                sessionOut.DestToken,
		DestWallet:               sessionOut.DestWallet,
		SettlementDestChain:      sessionOut.DestChain,
		SettlementDestToken:      sessionOut.DestToken,
		SettlementDestWallet:     sessionOut.DestWallet,
		ExpireTime:               sessionOut.ExpireTime,
		PaymentURL:               sessionOut.PaymentURL,
		PaymentCode:              sessionOut.PaymentCode,
	}

	// Populate fee breakdown (estimate based on quote source)
	// For stablecoin pairs, fees should be minimal (~0.3-1% per hop)
	feeEstimate := calculateFeeEstimate(quoteOut.QuotedAmount, selectedToken.Decimals, quoteOut.PriceSource)
	out.FeeBreakdown.PlatformFee = feeEstimate.platformFee
	out.FeeBreakdown.BridgeFee = feeEstimate.bridgeFee
	out.FeeBreakdown.TotalFee = feeEstimate.totalFee
	out.FeeBreakdown.TotalFeePercentage = feeEstimate.totalFeePercentage

	// Populate route info
	out.RouteInfo.Path = parseRoutePath(quoteOut.Route)
	out.RouteInfo.HopCount = len(out.RouteInfo.Path) - 1
	out.RouteInfo.IsDirect = len(out.RouteInfo.Path) <= 2
	out.RouteInfo.RouteSource = quoteOut.PriceSource
	out.PaymentInstruction.ChainID = sessionOut.PaymentInstruction.ChainID
	out.PaymentInstruction.To = sessionOut.PaymentInstruction.To
	out.PaymentInstruction.Value = sessionOut.PaymentInstruction.Value
	out.PaymentInstruction.Data = sessionOut.PaymentInstruction.Data
	out.PaymentInstruction.ProgramID = sessionOut.PaymentInstruction.ProgramID
	out.PaymentInstruction.DataBase58 = sessionOut.PaymentInstruction.DataBase58
	out.PaymentInstruction.DataBase64 = sessionOut.PaymentInstruction.DataBase64
	out.PaymentInstruction.ApprovalTo = sessionOut.PaymentInstruction.ApprovalTo
	out.PaymentInstruction.ApprovalHex = sessionOut.PaymentInstruction.ApprovalHex
	createPaymentTraceInfo(ctx, "create_payment.success",
		zap.String("payment_id", out.PaymentID),
		zap.String("quote_id", quoteOut.QuoteID),
		zap.String("pricing_type", string(pricingType)),
		zap.String("payer_selected_chain", out.PayerSelectedChain),
		zap.String("payer_selected_token_symbol", out.PayerSelectedTokenSymbol),
		zap.String("quoted_token_amount_atomic", out.QuotedTokenAmountAtomic),
		zap.String("settlement_dest_chain", out.SettlementDestChain),
		zap.String("settlement_dest_token", out.SettlementDestToken),
		zap.String("payment_instruction_to", out.PaymentInstruction.To),
		zap.String("payment_instruction_value", out.PaymentInstruction.Value),
		zap.Duration("total_latency", time.Since(startedAt)),
	)
	return out, nil
}

func createPaymentStageTimeoutFromEnv(envName string, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 5 * time.Second
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func resolveCreatePaymentExpiresAt(raw string, now time.Time) (time.Time, bool, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return now.Add(defaultCreatePaymentTTL), false, nil
	}
	if value == createPaymentUnlimitedValue {
		return createPaymentUnlimitedExpiryAt, true, nil
	}

	seconds, err := strconv.Atoi(value)
	if err != nil {
		return time.Time{}, false, domainerrors.BadRequest("expires_in must be a positive integer (seconds) or \"unlimited\"")
	}
	if seconds <= 0 {
		return time.Time{}, false, domainerrors.BadRequest("expires_in must be greater than 0 seconds")
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl < minCreatePaymentTTL {
		return time.Time{}, false, domainerrors.BadRequest(fmt.Sprintf("expires_in must be at least %d seconds", int(minCreatePaymentTTL.Seconds())))
	}
	if ttl > maxCreatePaymentTTL {
		return time.Time{}, false, domainerrors.BadRequest(fmt.Sprintf("expires_in must be at most %d seconds", int(maxCreatePaymentTTL.Seconds())))
	}
	return now.Add(ttl), false, nil
}

func isUnlimitedExpiryTime(value time.Time) bool {
	return !value.IsZero() && value.Equal(createPaymentUnlimitedExpiryAt)
}

func (u *CreatePaymentUsecase) GetPayment(ctx context.Context, paymentID uuid.UUID) (*CreatePaymentOutput, error) {
	if paymentID == uuid.Nil {
		return nil, domainerrors.BadRequest("payment id is required")
	}
	if u.sessionRepo == nil {
		return nil, domainerrors.InternalServerError("create payment read service is not configured")
	}

	session, err := u.sessionRepo.GetByID(ctx, paymentID)
	if err != nil || session == nil {
		return nil, domainerrors.NotFound("payment not found")
	}
	if session.Status == entities.PartnerPaymentSessionStatusPending && !isUnlimitedExpiryTime(session.ExpiresAt) && time.Now().UTC().After(session.ExpiresAt) {
		_ = u.sessionRepo.UpdateStatus(ctx, paymentID, entities.PartnerPaymentSessionStatusExpired)
		session.Status = entities.PartnerPaymentSessionStatusExpired
	}

	quotedSymbol := strings.TrimSpace(session.SelectedTokenSymbol)
	quotedAtomic := strings.TrimSpace(session.PaymentAmount)
	quotedDecimals := session.PaymentAmountDecimals
	quoteRate := stringValueOrDefault(session.QuoteRate, "")
	quoteSource := stringValueOrDefault(session.QuoteSource, "")
	quoteExpiresAt := session.ExpiresAt
	if session.QuoteExpiresAt != nil {
		quoteExpiresAt = *session.QuoteExpiresAt
	}

	if session.QuoteID != nil && u.quoteRepo != nil {
		if quote, qErr := u.quoteRepo.GetByID(ctx, *session.QuoteID); qErr == nil && quote != nil {
			if strings.TrimSpace(quote.SelectedTokenSymbol) != "" {
				quotedSymbol = strings.TrimSpace(quote.SelectedTokenSymbol)
			}
			if strings.TrimSpace(quote.QuotedAmount) != "" {
				quotedAtomic = strings.TrimSpace(quote.QuotedAmount)
			}
			if quote.SelectedTokenDecimals > 0 {
				quotedDecimals = quote.SelectedTokenDecimals
			}
			if strings.TrimSpace(quote.QuoteRate) != "" {
				quoteRate = strings.TrimSpace(quote.QuoteRate)
			}
			if strings.TrimSpace(quote.PriceSource) != "" {
				quoteSource = strings.TrimSpace(quote.PriceSource)
			}
			if !quote.ExpiresAt.IsZero() {
				quoteExpiresAt = quote.ExpiresAt
			}
		}
	}

	out := &CreatePaymentOutput{
		PaymentID:                session.ID.String(),
		MerchantID:               session.MerchantID.String(),
		Amount:                   session.PaymentAmount,
		InvoiceCurrency:          session.InvoiceCurrency,
		InvoiceAmount:            session.InvoiceAmount,
		PayerSelectedChain:       session.SelectedChainID,
		PayerSelectedToken:       session.SelectedTokenAddress,
		PayerSelectedTokenSymbol: session.SelectedTokenSymbol,
		QuotedTokenSymbol:        quotedSymbol,
		QuotedTokenAmount:        smallestUnitToDecimalString(quotedAtomic, quotedDecimals),
		QuotedTokenAmountAtomic:  quotedAtomic,
		QuotedTokenDecimals:      quotedDecimals,
		QuoteRate:                quoteRate,
		QuoteSource:              quoteSource,
		QuoteExpiresAt:           quoteExpiresAt,
		IsUnlimitedExpiry:        isUnlimitedExpiryTime(session.ExpiresAt),
		DestChain:                session.DestChain,
		DestToken:                session.DestToken,
		DestWallet:               session.DestWallet,
		SettlementDestChain:      session.DestChain,
		SettlementDestToken:      session.DestToken,
		SettlementDestWallet:     session.DestWallet,
		ExpireTime:               session.ExpiresAt,
		PaymentURL:               normalizePaymentURLWithSessionID(session.PaymentURL, session.ID),
		PaymentCode:              session.PaymentCode,
	}
	out.PaymentInstruction.ChainID = session.DestChain
	out.PaymentInstruction.To = session.InstructionTo
	out.PaymentInstruction.Value = session.InstructionValue
	out.PaymentInstruction.Data = session.InstructionDataHex
	out.PaymentInstruction.ProgramID = readInstructionProgramID(session)
	out.PaymentInstruction.DataBase58 = session.InstructionDataBase58
	out.PaymentInstruction.DataBase64 = session.InstructionDataBase64
	out.PaymentInstruction.ApprovalTo = session.InstructionApprovalTo
	out.PaymentInstruction.ApprovalHex = session.InstructionApprovalDataHex

	return out, nil
}

func stringValueOrDefault(input *string, fallback string) string {
	if input == nil {
		return fallback
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fallback
	}
	return value
}

func readInstructionProgramID(session *entities.PartnerPaymentSession) string {
	if session == nil {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(session.DestChain), "solana:") {
		if strings.TrimSpace(session.InstructionTo) != "" {
			return strings.TrimSpace(session.InstructionTo)
		}
		return strings.TrimSpace(session.DestToken)
	}
	return ""
}

func (u *CreatePaymentUsecase) resolveMerchantCreatePaymentConfig(ctx context.Context, merchant *entities.Merchant) merchantCreatePaymentConfig {
	if u.settlementRepo != nil && merchant != nil {
		profile, err := u.settlementRepo.GetByMerchantID(ctx, merchant.ID)
		if err == nil && profile != nil {
			return merchantCreatePaymentConfig{
				DestChain:         strings.TrimSpace(profile.DestChain),
				DestToken:         strings.TrimSpace(profile.DestToken),
				DestWallet:        strings.TrimSpace(profile.DestWallet),
				BridgeTokenSymbol: strings.TrimSpace(profile.BridgeTokenSymbol),
			}
		}
	}
	return merchantCreatePaymentConfig{}
}

func (u *CreatePaymentUsecase) resolveMerchantID(input *CreatePaymentInput) (uuid.UUID, error) {
	if input.MerchantContextID == uuid.Nil {
		return uuid.Nil, domainerrors.Forbidden("merchant context required")
	}
	if input.MerchantID != uuid.Nil && input.MerchantID != input.MerchantContextID {
		return uuid.Nil, domainerrors.Forbidden("merchant_id override is not allowed on this route")
	}
	return input.MerchantContextID, nil
}

func (u *CreatePaymentUsecase) resolveMerchantWallet(ctx context.Context, merchant *entities.Merchant, config merchantCreatePaymentConfig) (string, error) {
	if strings.TrimSpace(config.DestWallet) != "" {
		return strings.TrimSpace(config.DestWallet), nil
	}
	wallets, err := u.walletRepo.GetByUserID(ctx, merchant.UserID)
	if err != nil || len(wallets) == 0 {
		return "", domainerrors.BadRequest("merchant destination wallet is not configured")
	}
	for _, wallet := range wallets {
		if wallet != nil && wallet.IsPrimary && strings.TrimSpace(wallet.Address) != "" {
			return strings.TrimSpace(wallet.Address), nil
		}
	}
	if wallets[0] == nil || strings.TrimSpace(wallets[0].Address) == "" {
		return "", domainerrors.BadRequest("merchant destination wallet is not configured")
	}
	return strings.TrimSpace(wallets[0].Address), nil
}

func (u *CreatePaymentUsecase) createSyntheticSelectedTokenQuote(ctx context.Context, merchantID uuid.UUID, chainCAIP2 string, selectedToken *entities.Token, pricingType CreatePaymentPricingType, requestedAmount string, settlement *resolvedMerchantSettlementConfig, expiresAt time.Time) (*CreatePartnerQuoteOutput, error) {
	atomicAmount, err := convertToSmallestUnit(requestedAmount, selectedToken.Decimals)
	if err != nil {
		return nil, domainerrors.BadRequest(err.Error())
	}
	now := time.Now().UTC()
	invoiceCurrency := settlement.InvoiceToken.Symbol
	priceSource := "merchant-fixed-selected-token"
	if pricingType == CreatePaymentPricingTypePaymentTokenDyn {
		priceSource = "customer-input-selected-token"
	}
	if chainCAIP2 != settlement.DestChainCAIP2 {
		if strings.EqualFold(selectedToken.Symbol, settlement.BridgeTokenSymbol) {
			priceSource = fmt.Sprintf("cross-chain-bridge-token-direct-via-%s", strings.ToLower(settlement.BridgeTokenSymbol))
		} else {
			priceSource = fmt.Sprintf("cross-chain-normalized-via-%s", strings.ToLower(settlement.BridgeTokenSymbol))
		}
	}
	quote := &entities.PaymentQuote{
		ID:                    utils.GenerateUUIDv7(),
		MerchantID:            merchantID,
		InvoiceCurrency:       invoiceCurrency,
		InvoiceAmount:         atomicAmount,
		SelectedChainID:       chainCAIP2,
		SelectedTokenAddress:  selectedToken.ContractAddress,
		SelectedTokenSymbol:   selectedToken.Symbol,
		SelectedTokenDecimals: selectedToken.Decimals,
		QuotedAmount:          atomicAmount,
		QuoteRate:             quoteRateFromAtomicAmounts(atomicAmount, selectedToken.Decimals, atomicAmount, selectedToken.Decimals),
		PriceSource:           priceSource,
		Route:                 selectedToken.Symbol,
		SlippageBps:           0,
		RateTimestamp:         now,
		ExpiresAt:             expiresAt,
		Status:                entities.PaymentQuoteStatusActive,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := u.quoteRepo.Create(ctx, quote); err != nil {
		return nil, domainerrors.InternalServerError(fmt.Sprintf("failed to persist synthetic quote: %v", err))
	}
	return &CreatePartnerQuoteOutput{
		QuoteID:             quote.ID.String(),
		InvoiceCurrency:     quote.InvoiceCurrency,
		InvoiceAmount:       quote.InvoiceAmount,
		SelectedChain:       quote.SelectedChainID,
		SelectedToken:       quote.SelectedTokenAddress,
		SelectedTokenSymbol: quote.SelectedTokenSymbol,
		QuotedAmount:        quote.QuotedAmount,
		QuoteDecimals:       quote.SelectedTokenDecimals,
		QuoteRate:           quote.QuoteRate,
		PriceSource:         quote.PriceSource,
		Route:               quote.Route,
		SlippageBps:         quote.SlippageBps,
		RateTimestamp:       quote.RateTimestamp,
		QuoteExpiresAt:      quote.ExpiresAt,
	}, nil
}

func (u *CreatePaymentUsecase) createInvoiceCurrencyQuote(ctx context.Context, merchantID uuid.UUID, selectedChainCAIP2 string, selectedToken *entities.Token, settlement *resolvedMerchantSettlementConfig, requestedAmount string, destWallet string, expiresAt time.Time) (*CreatePartnerQuoteOutput, error) {
	invoiceAtomic, err := convertToSmallestUnit(requestedAmount, settlement.InvoiceToken.Decimals)
	if err != nil {
		return nil, domainerrors.BadRequest(err.Error())
	}
	createPaymentTraceInfo(ctx, "create_payment.invoice_quote_start",
		zap.String("merchant_id", merchantID.String()),
		zap.String("selected_chain", selectedChainCAIP2),
		zap.String("settlement_chain", settlement.DestChainCAIP2),
		zap.String("selected_token", selectedToken.Symbol),
		zap.String("invoice_token", settlement.InvoiceToken.Symbol),
		zap.String("invoice_amount_atomic", invoiceAtomic),
		zap.String("dest_wallet", destWallet),
	)
	if selectedChainCAIP2 == settlement.DestChainCAIP2 {
		createPaymentTraceDebug(ctx, "create_payment.invoice_quote_same_chain_path",
			zap.String("chain", selectedChainCAIP2),
			zap.String("pair", fmt.Sprintf("%s->%s", settlement.InvoiceToken.Symbol, selectedToken.Symbol)),
		)
		return u.quoteUC.CreateQuote(ctx, &CreatePartnerQuoteInput{
			MerchantID:        merchantID,
			InvoiceCurrency:   settlement.InvoiceToken.Symbol,
			InvoiceToken:      settlement.InvoiceToken.ContractAddress,
			InvoiceAmount:     invoiceAtomic,
			SelectedChain:     selectedChainCAIP2,
			SelectedToken:     selectedToken.ContractAddress,
			DestWallet:        destWallet,
			ExpiresAtOverride: &expiresAt,
		})
	}

	destBridgeAmount, routeSummary, _, err := u.resolveCrossChainBridgeAmount(ctx, merchantID, settlement, invoiceAtomic, destWallet)
	if err != nil {
		return nil, err
	}
	createPaymentTraceInfo(ctx, "create_payment.invoice_quote_bridge_amount_resolved",
		zap.String("dest_bridge_amount_atomic", destBridgeAmount),
		zap.String("bridge_route", routeSummary),
		zap.String("bridge_symbol", settlement.BridgeTokenSymbol),
	)
	if strings.EqualFold(selectedToken.Symbol, settlement.BridgeTokenSymbol) {
		createPaymentTraceDebug(ctx, "create_payment.invoice_quote_bridge_direct_source_token",
			zap.String("token_symbol", selectedToken.Symbol),
		)
		return u.createCompositeQuote(ctx, merchantID, selectedChainCAIP2, selectedToken, settlement.InvoiceToken.Symbol, settlement.InvoiceToken.Decimals, invoiceAtomic, destBridgeAmount, fmt.Sprintf("cross-chain-bridge-token-direct-via-%s", strings.ToLower(settlement.BridgeTokenSymbol)), routeSummary, expiresAt)
	}

	sourceBridgeToken, err := u.tokenRepo.GetBySymbol(ctx, settlement.BridgeTokenSymbol, selectedToken.ChainUUID)
	if err != nil || sourceBridgeToken == nil || !sourceBridgeToken.IsActive {
		return nil, domainerrors.BadRequest("bridge token is not supported on selected source chain")
	}
	requiredSourceAmount, sourceLegRoute, _, err := u.solveRequiredInputForTargetOutput(
		ctx,
		merchantID,
		selectedChainCAIP2,
		selectedToken,
		sourceBridgeToken,
		destBridgeAmount,
		destWallet,
	)
	if err != nil {
		return nil, annotateCreatePaymentEstimateError("unable to estimate source amount from selected token to bridge token", err)
	}
	createPaymentTraceInfo(ctx, "create_payment.invoice_quote_source_amount_resolved",
		zap.String("source_token", selectedToken.Symbol),
		zap.String("bridge_token", sourceBridgeToken.Symbol),
		zap.String("required_source_amount_atomic", requiredSourceAmount),
		zap.String("source_leg_route", sourceLegRoute),
	)

	return u.createCompositeQuote(
		ctx,
		merchantID,
		selectedChainCAIP2,
		selectedToken,
		settlement.InvoiceToken.Symbol,
		settlement.InvoiceToken.Decimals,
		invoiceAtomic,
		requiredSourceAmount,
		fmt.Sprintf("cross-chain-normalized-via-%s", strings.ToLower(settlement.BridgeTokenSymbol)),
		fmt.Sprintf("%s | %s", sourceLegRoute, routeSummary),
		expiresAt,
	)
}

func (u *CreatePaymentUsecase) resolveCrossChainBridgeAmount(ctx context.Context, merchantID uuid.UUID, settlement *resolvedMerchantSettlementConfig, invoiceAtomic string, destWallet string) (string, string, string, error) {
	createPaymentTraceInfo(ctx, "create_payment.bridge_amount_start",
		zap.String("merchant_id", merchantID.String()),
		zap.String("dest_chain", settlement.DestChainCAIP2),
		zap.String("invoice_token_symbol", settlement.InvoiceToken.Symbol),
		zap.String("dest_bridge_token_symbol", settlement.DestBridgeToken.Symbol),
		zap.String("invoice_atomic", invoiceAtomic),
		zap.String("dest_wallet", destWallet),
	)
	if strings.EqualFold(settlement.InvoiceToken.ContractAddress, settlement.DestBridgeToken.ContractAddress) {
		createPaymentTraceDebug(ctx, "create_payment.bridge_amount_direct",
			zap.String("bridge_amount_atomic", invoiceAtomic),
			zap.String("route", fmt.Sprintf("%s->%s", settlement.InvoiceToken.Symbol, settlement.DestBridgeToken.Symbol)),
		)
		return invoiceAtomic, fmt.Sprintf("%s->%s", settlement.InvoiceToken.Symbol, settlement.DestBridgeToken.Symbol), fmt.Sprintf("cross-chain-bridge-token-direct-via-%s", strings.ToLower(settlement.BridgeTokenSymbol)), nil
	}
	requiredBridgeAmount, route, source, err := u.solveRequiredInputForTargetOutput(
		ctx,
		merchantID,
		settlement.DestChainCAIP2,
		settlement.DestBridgeToken,
		settlement.InvoiceToken,
		invoiceAtomic,
		destWallet,
	)
	if err != nil {
		createPaymentTraceWarn(ctx, "create_payment.bridge_amount_failed",
			zap.String("dest_chain", settlement.DestChainCAIP2),
			zap.String("pair", fmt.Sprintf("%s->%s", settlement.DestBridgeToken.Symbol, settlement.InvoiceToken.Symbol)),
			zap.String("target_invoice_atomic", invoiceAtomic),
			zap.Error(err),
		)
		return "", "", "", annotateCreatePaymentEstimateError("unable to estimate destination bridge amount from invoice token", err)
	}
	createPaymentTraceInfo(ctx, "create_payment.bridge_amount_success",
		zap.String("required_bridge_amount_atomic", requiredBridgeAmount),
		zap.String("route", route),
		zap.String("price_source", source),
	)
	return requiredBridgeAmount, route, source, nil
}

func (u *CreatePaymentUsecase) solveRequiredInputForTargetOutput(
	ctx context.Context,
	merchantID uuid.UUID,
	chainCAIP2 string,
	inputToken *entities.Token,
	outputToken *entities.Token,
	targetOutputAtomic string,
	destWallet string,
) (string, string, string, error) {
	startedAt := time.Now()
	ctx = withQuoteRequestCache(ctx)
	target, ok := new(big.Int).SetString(strings.TrimSpace(targetOutputAtomic), 10)
	if !ok || target == nil || target.Sign() <= 0 {
		return "", "", "", domainerrors.BadRequest("target output amount must be a positive integer string")
	}
	probeContext := fmt.Sprintf("chain=%s pair=%s->%s", chainCAIP2, strings.TrimSpace(inputToken.Symbol), strings.TrimSpace(outputToken.Symbol))
	createPaymentTraceInfo(ctx, "create_payment.inverse_estimation_start",
		zap.String("probe_context", probeContext),
		zap.String("target_output_atomic", target.String()),
		zap.String("target_output", smallestUnitToDecimalString(target.String(), outputToken.Decimals)),
		zap.String("input_token_address", inputToken.ContractAddress),
		zap.String("output_token_address", outputToken.ContractAddress),
	)

	inversePreviewHint := ""
	if !preferDryRunQuote(ctx) {
		if inverseUC, ok := u.quoteUC.(createPaymentQuoteInversePreviewEngine); ok {
			inverseOut, inverseErr := inverseUC.PreviewRequiredInputForOutput(ctx, &PreviewRequiredInputForOutputInput{
				MerchantID:         merchantID,
				SelectedChain:      chainCAIP2,
				InputToken:         inputToken.ContractAddress,
				OutputToken:        outputToken.ContractAddress,
				TargetOutputAmount: target.String(),
			})
			if inverseErr != nil {
				inversePreviewHint = strings.TrimSpace(inverseErr.Error())
				createPaymentTraceWarn(ctx, "create_payment.inverse_preview_failed",
					zap.String("probe_context", probeContext),
					zap.Error(inverseErr),
				)
			}
			if inverseErr == nil && inverseOut != nil {
				required, parsed := new(big.Int).SetString(strings.TrimSpace(inverseOut.RequiredInputAmount), 10)
				if parsed && required != nil && required.Sign() > 0 {
					createPaymentTraceInfo(ctx, "create_payment.inverse_preview_success",
						zap.String("probe_context", probeContext),
						zap.String("required_input_atomic", required.String()),
						zap.String("route", strings.TrimSpace(inverseOut.Route)),
						zap.String("price_source", strings.TrimSpace(inverseOut.PriceSource)),
						zap.Duration("latency", time.Since(startedAt)),
					)
					return required.String(), strings.TrimSpace(inverseOut.Route), strings.TrimSpace(inverseOut.PriceSource), nil
				}
			}
		}
	} else {
		createPaymentTraceDebug(ctx, "create_payment.inverse_preview_skipped_dry_run",
			zap.String("probe_context", probeContext),
		)
	}

	lastProbeZeroReason := ""
	lastProbeError := ""
	upperBoundProbes := 0
	binarySearchProbes := 0

	quoteForInput := func(stage string, iteration int, inputAmount *big.Int) (*big.Int, string, string, error) {
		if inputAmount == nil || inputAmount.Sign() <= 0 {
			return nil, "", "", domainerrors.BadRequest("input amount must be positive")
		}
		quoteInput := &CreatePartnerQuoteInput{
			MerchantID:      merchantID,
			InvoiceCurrency: inputToken.Symbol,
			InvoiceToken:    inputToken.ContractAddress,
			InvoiceAmount:   inputAmount.String(),
			SelectedChain:   chainCAIP2,
			SelectedToken:   outputToken.ContractAddress,
			DestWallet:      destWallet,
		}
		var quoteOut *CreatePartnerQuoteOutput
		var err error
		persistedQuote := false
		if previewUC, ok := u.quoteUC.(createPaymentQuotePreviewEngine); ok {
			quoteOut, err = previewUC.PreviewQuote(ctx, quoteInput)
		} else {
			quoteOut, err = u.quoteUC.CreateQuote(ctx, quoteInput)
			persistedQuote = true
		}
		if err != nil {
			if treatAsZero, reason := shouldTreatQuoteProbeAsZero(err); treatAsZero {
				// Tiny probe amounts can legitimately be unquotable on quoter paths.
				// Treat as zero output so the search can continue to larger amounts.
				if strings.TrimSpace(reason) != "" {
					lastProbeZeroReason = strings.TrimSpace(reason)
				}
				createPaymentTraceDebug(ctx, "create_payment.inverse_estimation_probe_zero",
					zap.String("probe_context", probeContext),
					zap.String("stage", stage),
					zap.Int("iteration", iteration),
					zap.String("probe_input_atomic", inputAmount.String()),
					zap.String("reason", reason),
				)
				return big.NewInt(0), "", "", nil
			}
			lastProbeError = strings.TrimSpace(err.Error())
			createPaymentTraceWarn(ctx, "create_payment.inverse_estimation_probe_failed",
				zap.String("probe_context", probeContext),
				zap.String("stage", stage),
				zap.Int("iteration", iteration),
				zap.String("probe_input_atomic", inputAmount.String()),
				zap.Error(err),
			)
			return nil, "", "", err
		}
		if quoteOut == nil {
			return nil, "", "", domainerrors.InternalServerError("invalid temporary quote output")
		}
		if persistedQuote {
			quoteID, parseErr := uuid.Parse(quoteOut.QuoteID)
			if parseErr == nil {
				_ = u.quoteRepo.UpdateStatus(ctx, quoteID, entities.PaymentQuoteStatusCancelled)
			}
		}
		quotedOut, parsed := new(big.Int).SetString(strings.TrimSpace(quoteOut.QuotedAmount), 10)
		if !parsed || quotedOut == nil || quotedOut.Sign() < 0 {
			return nil, "", "", domainerrors.InternalServerError("invalid temporary quote amount")
		}
		createPaymentTraceDebug(ctx, "create_payment.inverse_estimation_probe_success",
			zap.String("probe_context", probeContext),
			zap.String("stage", stage),
			zap.Int("iteration", iteration),
			zap.String("probe_input_atomic", inputAmount.String()),
			zap.String("quoted_output_atomic", quotedOut.String()),
			zap.String("quoted_output", smallestUnitToDecimalString(quotedOut.String(), outputToken.Decimals)),
			zap.String("route", strings.TrimSpace(quoteOut.Route)),
			zap.String("price_source", strings.TrimSpace(quoteOut.PriceSource)),
		)
		return quotedOut, quoteOut.Route, quoteOut.PriceSource, nil
	}

	low := big.NewInt(1)
	high := new(big.Int).Set(target)
	bestRoute := ""
	bestSource := ""
	foundUpperBound := false
	lastQuotedOut := big.NewInt(0)

	// Anchor probe narrows the initial interval substantially for near-linear pools.
	// This cuts binary-search probes while preserving exact integer output checks.
	anchorOut, route, source, anchorErr := quoteForInput("anchor", 0, target)
	if anchorErr == nil && anchorOut != nil && anchorOut.Sign() > 0 {
		bestRoute = route
		bestSource = source
		createPaymentTraceDebug(ctx, "create_payment.inverse_estimation_anchor_probe",
			zap.String("probe_context", probeContext),
			zap.String("anchor_input_atomic", target.String()),
			zap.String("anchor_output_atomic", anchorOut.String()),
		)
		if anchorOut.Cmp(target) == 0 {
			createPaymentTraceInfo(ctx, "create_payment.inverse_estimation_anchor_exact",
				zap.String("probe_context", probeContext),
				zap.String("required_input_atomic", target.String()),
				zap.String("route", bestRoute),
				zap.String("price_source", bestSource),
				zap.Duration("latency", time.Since(startedAt)),
			)
			return target.String(), bestRoute, bestSource, nil
		}

		numerator := new(big.Int).Mul(target, target)
		guess := new(big.Int).Div(numerator, anchorOut)
		if new(big.Int).Mod(numerator, anchorOut).Sign() > 0 {
			guess = guess.Add(guess, big.NewInt(1))
		}
		if guess.Sign() <= 0 {
			guess = big.NewInt(1)
		}

		low = new(big.Int).Div(guess, big.NewInt(2))
		if low.Sign() <= 0 {
			low = big.NewInt(1)
		}
		high = new(big.Int).Mul(guess, big.NewInt(2))
		if high.Sign() <= 0 {
			high = big.NewInt(1)
		}
	}

	for i := 0; i < maxCreatePaymentUpperBoundDoublings; i++ {
		upperBoundProbes++
		quotedOut, route, source, err := quoteForInput("upper_bound", i, high)
		if err != nil {
			return "", "", "", err
		}
		if quotedOut != nil && quotedOut.Sign() >= 0 {
			lastQuotedOut = new(big.Int).Set(quotedOut)
		}
		if quotedOut.Cmp(target) >= 0 {
			bestRoute = route
			bestSource = source
			foundUpperBound = true
			createPaymentTraceDebug(ctx, "create_payment.inverse_estimation_upper_bound_found",
				zap.String("probe_context", probeContext),
				zap.Int("iteration", i),
				zap.String("high_atomic", high.String()),
				zap.String("quoted_output_atomic", quotedOut.String()),
			)
			break
		}
		high = new(big.Int).Mul(high, big.NewInt(2))
	}

	if !foundUpperBound {
		createPaymentTraceWarn(ctx, "create_payment.inverse_estimation_upper_bound_missing",
			zap.String("probe_context", probeContext),
			zap.String("last_probe_zero_reason", lastProbeZeroReason),
			zap.String("last_probe_error", lastProbeError),
			zap.String("last_quoted_output_atomic", lastQuotedOut.String()),
			zap.String("target_output_atomic", target.String()),
			zap.String("inverse_hint", inversePreviewHint),
			zap.Int("upper_bound_probes", upperBoundProbes),
			zap.Duration("latency", time.Since(startedAt)),
		)
		if lastProbeZeroReason != "" {
			return "", "", "", domainerrors.BadRequest(fmt.Sprintf("unable to estimate required source amount for requested invoice: %s (%s)", lastProbeZeroReason, probeContext))
		}
		if lastProbeError != "" {
			return "", "", "", domainerrors.BadRequest(fmt.Sprintf("unable to estimate required source amount for requested invoice: %s (%s)", lastProbeError, probeContext))
		}
		if lastQuotedOut != nil && lastQuotedOut.Sign() > 0 {
			targetHuman := smallestUnitToDecimalString(target.String(), outputToken.Decimals)
			maxHuman := smallestUnitToDecimalString(lastQuotedOut.String(), outputToken.Decimals)
			outputSymbol := strings.TrimSpace(outputToken.Symbol)
			if outputSymbol == "" {
				outputSymbol = "output_token"
			}
			if inversePreviewHint != "" {
				return "", "", "", domainerrors.BadRequest(fmt.Sprintf("unable to estimate required source amount for requested invoice: requested amount likely exceeds available route liquidity (%s, target_output=%s %s, max_preview_output=%s %s, target_output_atomic=%s, max_quoted_output_atomic=%s, inverse_hint=%s). Reduce requested_amount below %s %s or increase pool liquidity.", probeContext, targetHuman, outputSymbol, maxHuman, outputSymbol, target.String(), lastQuotedOut.String(), inversePreviewHint, maxHuman, outputSymbol))
			}
			return "", "", "", domainerrors.BadRequest(fmt.Sprintf("unable to estimate required source amount for requested invoice: requested amount likely exceeds available route liquidity (%s, target_output=%s %s, max_preview_output=%s %s, target_output_atomic=%s, max_quoted_output_atomic=%s). Reduce requested_amount below %s %s or increase pool liquidity.", probeContext, targetHuman, outputSymbol, maxHuman, outputSymbol, target.String(), lastQuotedOut.String(), maxHuman, outputSymbol))
		}
		if inversePreviewHint != "" {
			return "", "", "", domainerrors.BadRequest(fmt.Sprintf("unable to estimate required source amount for requested invoice: %s (%s)", inversePreviewHint, probeContext))
		}
		return "", "", "", domainerrors.BadRequest("unable to estimate required source amount for requested invoice")
	}

	for i := 0; i < maxCreatePaymentBinarySearchIterations && low.Cmp(high) < 0; i++ {
		binarySearchProbes++
		mid := new(big.Int).Add(low, high)
		mid.Div(mid, big.NewInt(2))
		if mid.Sign() <= 0 {
			mid = big.NewInt(1)
		}

		quotedOut, route, source, err := quoteForInput("binary_search", i, mid)
		if err != nil {
			return "", "", "", err
		}
		if quotedOut.Cmp(target) >= 0 {
			high = mid
			bestRoute = route
			bestSource = source
		} else {
			low = new(big.Int).Add(mid, big.NewInt(1))
		}
	}

	createPaymentTraceInfo(ctx, "create_payment.inverse_estimation_success",
		zap.String("probe_context", probeContext),
		zap.String("required_input_atomic", high.String()),
		zap.String("required_input", smallestUnitToDecimalString(high.String(), inputToken.Decimals)),
		zap.String("target_output_atomic", target.String()),
		zap.String("target_output", smallestUnitToDecimalString(target.String(), outputToken.Decimals)),
		zap.String("route", bestRoute),
		zap.String("price_source", bestSource),
		zap.Int("upper_bound_probes", upperBoundProbes),
		zap.Int("binary_search_probes", binarySearchProbes),
		zap.Duration("latency", time.Since(startedAt)),
	)
	return high.String(), bestRoute, bestSource, nil
}

func shouldTreatQuoteProbeAsZero(err error) (bool, string) {
	if err == nil {
		return false, ""
	}
	var appErr *domainerrors.AppError
	if !errors.As(err, &appErr) {
		return false, ""
	}
	if appErr.Status != 400 {
		return false, ""
	}
	message := strings.ToLower(strings.TrimSpace(appErr.Message))
	if message == "" {
		return false, ""
	}
	if strings.Contains(message, "placeholder quote") {
		return true, "accurate quote unavailable for selected pair"
	}
	if strings.Contains(message, "no usable v3 fee tier found") {
		return true, "selected token pair has no usable route on selected chain"
	}
	if strings.Contains(message, "amountout <= 0") {
		return true, "selected token pair returned zero output for probed amounts"
	}
	return false, ""
}

func annotateCreatePaymentEstimateError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return err
	}

	var appErr *domainerrors.AppError
	if errors.As(err, &appErr) {
		message := strings.TrimSpace(appErr.Message)
		if message == "" {
			message = strings.TrimSpace(err.Error())
		}
		return domainerrors.NewAppError(appErr.Status, appErr.Code, fmt.Sprintf("%s: %s", prefix, message), appErr.Err)
	}

	return domainerrors.BadRequest(fmt.Sprintf("%s: %s", prefix, strings.TrimSpace(err.Error())))
}

func (u *CreatePaymentUsecase) createCompositeQuote(ctx context.Context, merchantID uuid.UUID, selectedChainCAIP2 string, selectedToken *entities.Token, invoiceCurrency string, invoiceDecimals int, invoiceAtomic string, quotedAmount string, priceSource string, route string, expiresAt time.Time) (*CreatePartnerQuoteOutput, error) {
	now := time.Now().UTC()
	quote := &entities.PaymentQuote{
		ID:                    utils.GenerateUUIDv7(),
		MerchantID:            merchantID,
		InvoiceCurrency:       invoiceCurrency,
		InvoiceAmount:         invoiceAtomic,
		SelectedChainID:       selectedChainCAIP2,
		SelectedTokenAddress:  selectedToken.ContractAddress,
		SelectedTokenSymbol:   selectedToken.Symbol,
		SelectedTokenDecimals: selectedToken.Decimals,
		QuotedAmount:          quotedAmount,
		QuoteRate:             quoteRateFromAtomicAmounts(quotedAmount, selectedToken.Decimals, invoiceAtomic, invoiceDecimals),
		PriceSource:           priceSource,
		Route:                 route,
		SlippageBps:           partnerQuoteSlippage,
		RateTimestamp:         now,
		ExpiresAt:             expiresAt,
		Status:                entities.PaymentQuoteStatusActive,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := u.quoteRepo.Create(ctx, quote); err != nil {
		return nil, domainerrors.InternalServerError(fmt.Sprintf("failed to persist composite quote: %v", err))
	}
	return &CreatePartnerQuoteOutput{
		QuoteID:             quote.ID.String(),
		InvoiceCurrency:     quote.InvoiceCurrency,
		InvoiceAmount:       quote.InvoiceAmount,
		SelectedChain:       quote.SelectedChainID,
		SelectedToken:       quote.SelectedTokenAddress,
		SelectedTokenSymbol: quote.SelectedTokenSymbol,
		QuotedAmount:        quote.QuotedAmount,
		QuoteDecimals:       quote.SelectedTokenDecimals,
		QuoteRate:           quote.QuoteRate,
		PriceSource:         quote.PriceSource,
		Route:               quote.Route,
		SlippageBps:         quote.SlippageBps,
		RateTimestamp:       quote.RateTimestamp,
		QuoteExpiresAt:      quote.ExpiresAt,
	}, nil
}

func (u *CreatePaymentUsecase) resolveMerchantSettlementConfig(ctx context.Context, config merchantCreatePaymentConfig) (*resolvedMerchantSettlementConfig, error) {
	if strings.TrimSpace(config.DestChain) == "" {
		return nil, domainerrors.BadRequest("merchant dest_chain is not configured")
	}
	if strings.TrimSpace(config.DestToken) == "" {
		return nil, domainerrors.BadRequest("merchant dest_token is not configured")
	}
	destChainID, destChainCAIP2, err := u.chainResolver.ResolveFromAny(ctx, config.DestChain)
	if err != nil {
		return nil, domainerrors.BadRequest(fmt.Sprintf("invalid merchant dest_chain: %v", err))
	}
	destToken, err := u.tokenRepo.GetByAddress(ctx, config.DestToken, destChainID)
	if err != nil || destToken == nil || !destToken.IsActive {
		return nil, domainerrors.BadRequest("merchant dest_token is not supported")
	}
	invoiceToken := destToken
	bridgeSymbol := strings.TrimSpace(config.BridgeTokenSymbol)
	if bridgeSymbol == "" {
		bridgeSymbol = "USDC"
	}
	destBridgeToken, err := u.tokenRepo.GetBySymbol(ctx, bridgeSymbol, destChainID)
	if err != nil || destBridgeToken == nil || !destBridgeToken.IsActive {
		return nil, domainerrors.BadRequest("merchant bridge token is not supported on dest_chain")
	}
	return &resolvedMerchantSettlementConfig{
		InvoiceToken:      invoiceToken,
		DestChainID:       destChainID,
		DestChainCAIP2:    destChainCAIP2,
		DestToken:         destToken,
		DestWallet:        strings.TrimSpace(config.DestWallet),
		BridgeTokenSymbol: bridgeSymbol,
		DestBridgeToken:   destBridgeToken,
	}, nil
}

func smallestUnitToDecimalString(amount string, decimals int) string {
	raw := strings.TrimSpace(amount)
	if raw == "" {
		return "0"
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return raw
		}
	}
	if decimals <= 0 {
		return strings.TrimLeft(raw, "0")
	}
	if len(raw) <= decimals {
		raw = strings.Repeat("0", decimals-len(raw)+1) + raw
	}
	split := len(raw) - decimals
	whole := strings.TrimLeft(raw[:split], "0")
	if whole == "" {
		whole = "0"
	}
	frac := strings.TrimRight(raw[split:], "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

func quoteRateFromAtomicAmounts(quotedAmount string, quotedDecimals int, invoiceAmount string, invoiceDecimals int) string {
	quoted, ok := new(big.Int).SetString(strings.TrimSpace(quotedAmount), 10)
	if !ok || quoted == nil || quoted.Sign() <= 0 {
		return "0"
	}
	invoice, ok := new(big.Int).SetString(strings.TrimSpace(invoiceAmount), 10)
	if !ok || invoice == nil || invoice.Sign() <= 0 {
		return "0"
	}
	return formatNormalizedTokenRatio(quoted, quotedDecimals, invoice, invoiceDecimals, 18)
}

// calculateFeeEstimate estimates fees based on quoted amount and route
func calculateFeeEstimate(quotedAmountAtomic string, decimals int, priceSource string) (feeEstimate struct {
	platformFee        string
	bridgeFee          string
	totalFee           string
	totalFeePercentage string
}) {
	// Default estimates (will be refined based on actual fee calculation)
	// For stablecoin pairs with multi-hop, estimate ~0.5-1% total fees
	quotedAmount, ok := new(big.Int).SetString(strings.TrimSpace(quotedAmountAtomic), 10)
	if !ok || quotedAmount == nil || quotedAmount.Sign() <= 0 {
		return
	}

	// Estimate platform fee: 0.3% (typical Uniswap V3 fee)
	platformFeeBps := big.NewInt(30) // 0.3%
	platformFee := new(big.Int).Mul(quotedAmount, platformFeeBps)
	platformFee.Div(platformFee, big.NewInt(10000))

	// Estimate bridge fee: 0.1-0.5% depending on bridge
	bridgeFeeBps := big.NewInt(20) // 0.2%
	if strings.Contains(strings.ToLower(priceSource), "cross-chain") {
		bridgeFeeBps = big.NewInt(30) // 0.3% for cross-chain
	}
	bridgeFee := new(big.Int).Mul(quotedAmount, bridgeFeeBps)
	bridgeFee.Div(bridgeFee, big.NewInt(10000))

	// Total fee
	totalFee := new(big.Int).Add(platformFee, bridgeFee)
	totalFeeBps := new(big.Int).Add(platformFeeBps, bridgeFeeBps)

	// Convert to decimal string
	feeEstimate.platformFee = smallestUnitToDecimalString(platformFee.String(), decimals)
	feeEstimate.bridgeFee = smallestUnitToDecimalString(bridgeFee.String(), decimals)
	feeEstimate.totalFee = smallestUnitToDecimalString(totalFee.String(), decimals)
	feeEstimate.totalFeePercentage = fmt.Sprintf("%.2f%%", float64(totalFeeBps.Int64())/100.0)

	return
}

// parseRoutePath parses route string (e.g., "IDRT->USDC->IDRX") into path array
func parseRoutePath(route string) []string {
	if route == "" {
		return []string{}
	}
	parts := strings.Split(route, "->")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}
