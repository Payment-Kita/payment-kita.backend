package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"payment-kita.backend/internal/config"
	"payment-kita.backend/internal/domain/services"
	"payment-kita.backend/internal/infrastructure/blockchain"
	"payment-kita.backend/internal/infrastructure/jobs"
	"payment-kita.backend/internal/infrastructure/repositories"
	servicesimpl "payment-kita.backend/internal/infrastructure/services"
	"payment-kita.backend/internal/interfaces/http/handlers"
	"payment-kita.backend/internal/interfaces/http/middleware"
	"payment-kita.backend/internal/usecases"
	"payment-kita.backend/pkg/jwt"
	"payment-kita.backend/pkg/logger"
	"payment-kita.backend/pkg/redis"
)

var (
	loadDotenv = godotenv.Load
	loadCfg    = config.Load
	initLog    = logger.Init
	initRedis  = redis.Init
	openDB     = func(dsn string) (*gorm.DB, error) {
		return gorm.Open(postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		}), &gorm.Config{
			PrepareStmt: false,
		})
	}
	newSessionStore = redis.NewSessionStore
	runServer       = func(r *gin.Engine, port string) error { return r.Run(":" + port) }
	getStdDB        = func(db *gorm.DB) (*sql.DB, error) { return db.DB() }
)

func main() {
	if err := runMainProcess(); err != nil {
		log.Fatal(err)
	}
}

func runMainProcess() error {
	// Load .env file
	if err := loadDotenv(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := loadCfg()

	// Initialize Logger
	initLog(cfg.Server.Env)
	logger.Info(context.Background(), "Logger initialized", zap.String("env", cfg.Server.Env))

	// Initialize Redis
	if err := initRedis(cfg.Redis.URL, cfg.Redis.Username, cfg.Redis.PASSWORD); err != nil {
		logger.Error(context.Background(), "Failed to initialize Redis", zap.Error(err))
		return fmt.Errorf("failed to initialize redis: %w", err)
	}
	logger.Info(context.Background(), "Redis initialized")

	// Set Gin mode
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Connect to database using GORM
	dsn := cfg.Database.URL()
	db, err := openDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := getStdDB(db)
	if err != nil {
		return fmt.Errorf("failed to get generic database object: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		log.Printf("⚠️ Database not available: %v (endpoints will return errors)", err)
	} else {
		log.Println("✅ Connected to PostgreSQL via GORM")
	}

	// Initialize JWT service
	jwtService := jwt.NewJWTService(
		cfg.JWT.Secret,
		cfg.JWT.AccessExpiry,
		cfg.JWT.RefreshExpiry,
	)

	// Initialize repositories
	userRepo := repositories.NewUserRepository(db)
	emailVerifRepo := repositories.NewEmailVerificationRepository(db)
	merchantRepo := repositories.NewMerchantRepository(db)
	paymentRepo := repositories.NewPaymentRepository(db)
	paymentEventRepo := repositories.NewPaymentEventRepository(db)
	walletRepo := repositories.NewWalletRepository(db)
	chainRepo := repositories.NewChainRepository(db)
	tokenRepo := repositories.NewTokenRepository(db, chainRepo)
	paymentBridgeRepo := repositories.NewPaymentBridgeRepository(db)
	bridgeConfigRepo := repositories.NewBridgeConfigRepository(db)
	feeConfigRepo := repositories.NewFeeConfigRepository(db)
	routePolicyRepo := repositories.NewRoutePolicyRepository(db)
	stargateConfigRepo := repositories.NewStargateConfigRepository(db)
	smartContractRepo := repositories.NewSmartContractRepository(db, chainRepo)
	paymentRequestRepo := repositories.NewPaymentRequestRepository(db)
	paymentQuoteRepo := repositories.NewPaymentQuoteRepository(db)
	settlementProfileRepo := repositories.NewMerchantSettlementProfileRepository(db)
	teamRepo := repositories.NewTeamRepository(db)
	apiKeyRepo := repositories.NewApiKeyRepository(db)
	webhookLogRepo := repositories.NewGormWebhookLogRepository(db)
	auditLogRepo := repositories.NewAuditLogRepository(db)
	resolveAuditRepo := repositories.NewResolveAuditRepository(db)
	uow := repositories.NewUnitOfWork(db)

	// Initialize Session Store
	sessionStore, err := newSessionStore(cfg.Security.SessionEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}

	// Initialize JWEService for Partner Flow
	jweKey, _ := hex.DecodeString(cfg.Security.JweMasterKey)
	jweService, err := services.NewJWEService(jweKey)
	if err != nil {
		return fmt.Errorf("failed to initialize jwe service: %w", err)
	}
	hmacService := servicesimpl.NewHMACService()
	complianceService := services.NewComplianceService(80)

	// Initialize blockchain client factory
	clientFactory := blockchain.NewClientFactory()

	// Initialize usecases
	authUsecase := usecases.NewAuthUsecase(userRepo, emailVerifRepo, walletRepo, chainRepo, merchantRepo, uow, jwtService)
	// ApiKeyUsecase needs Config for Encryption Key
	apiKeyUsecase := usecases.NewApiKeyUsecase(apiKeyRepo, userRepo, cfg.Security.ApiKeyEncryptionKey)
	paymentUsecase := usecases.NewPaymentUsecase(paymentRepo, paymentEventRepo, walletRepo, merchantRepo, smartContractRepo, chainRepo, tokenRepo, bridgeConfigRepo, feeConfigRepo, routePolicyRepo, uow, clientFactory)
	// PaymentAppUsecase needs PaymentUsecase, UserRepo, WalletRepo, ChainRepo
	paymentAppUsecase := usecases.NewPaymentAppUsecase(paymentUsecase, userRepo, walletRepo, chainRepo)
	merchantUsecase := usecases.NewMerchantUsecase(merchantRepo, userRepo)
	walletUsecase := usecases.NewWalletUsecase(walletRepo, userRepo, chainRepo)

	paymentRequestUsecase := usecases.NewPaymentRequestUsecase(paymentRequestRepo, merchantRepo, walletRepo, chainRepo, smartContractRepo, tokenRepo, jweService)
	partnerQuoteUsecase := usecases.NewPartnerQuoteUsecase(paymentQuoteRepo, tokenRepo, chainRepo, paymentUsecase)
	partnerPaymentSessionUsecase := usecases.NewPartnerPaymentSessionUsecase(
		paymentQuoteRepo,
		repositories.NewPartnerPaymentSessionRepository(db),
		paymentRequestRepo,
		smartContractRepo,
		tokenRepo,
		chainRepo,
		merchantRepo,
		uow,
		jweService,
		paymentRequestUsecase,
		paymentUsecase,
		os.Getenv("PARTNER_CHECKOUT_BASE_URL"),
	)
	createPaymentUsecase := usecases.NewCreatePaymentUsecase(
		merchantRepo,
		settlementProfileRepo,
		walletRepo,
		tokenRepo,
		chainRepo,
		paymentQuoteRepo,
		repositories.NewPartnerPaymentSessionRepository(db),
		partnerQuoteUsecase,
		partnerPaymentSessionUsecase,
	)
	// Step 3: Webhook Delivery Engine
	webhookDispatcher := usecases.NewWebhookDispatcher(webhookLogRepo, merchantRepo, hmacService)
	webhookJob := jobs.NewWebhookDeliveryJob(webhookLogRepo, webhookDispatcher)

	webhookUsecase := usecases.NewWebhookUsecase(paymentRepo, paymentEventRepo, paymentRequestRepo, repositories.NewPartnerPaymentSessionRepository(db), merchantRepo, webhookLogRepo, webhookDispatcher, uow)
	onchainAdapterUsecase := usecases.NewOnchainAdapterUsecase(chainRepo, smartContractRepo, clientFactory, cfg.Blockchain.OwnerPrivateKey)
	contractConfigAuditUsecase := usecases.NewContractConfigAuditUsecase(chainRepo, smartContractRepo, clientFactory)
	crosschainConfigUsecase := usecases.NewCrosschainConfigUsecase(chainRepo, tokenRepo, smartContractRepo, clientFactory, onchainAdapterUsecase)
	routeErrorUsecase := usecases.NewRouteErrorUsecase(chainRepo, smartContractRepo, clientFactory)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authUsecase, sessionStore)
	paymentHandler := handlers.NewPaymentHandler(paymentUsecase)
	merchantHandler := handlers.NewMerchantHandler(merchantUsecase)
	walletHandler := handlers.NewWalletHandler(walletUsecase)
	chainHandler := handlers.NewChainHandler(chainRepo)
	tokenHandler := handlers.NewTokenHandler(tokenRepo, chainRepo, paymentUsecase)
	smartContractHandler := handlers.NewSmartContractHandler(smartContractRepo, chainRepo)
	paymentRequestHandler := handlers.NewPaymentRequestHandler(paymentRequestUsecase)
	webhookHandler := handlers.NewWebhookHandler(webhookUsecase)
	adminHandler := handlers.NewAdminHandler(userRepo, merchantRepo, paymentRepo, settlementProfileRepo)
	adminMerchantSettlementHandler := handlers.NewAdminMerchantSettlementHandler(merchantRepo, settlementProfileRepo, chainRepo, tokenRepo)
	merchantSettlementHandler := handlers.NewMerchantSettlementHandler(merchantRepo, settlementProfileRepo, chainRepo, tokenRepo)
	teamHandler := handlers.NewTeamHandler(teamRepo)
	apiKeyHandler := handlers.NewApiKeyHandler(apiKeyUsecase)             // Added
	paymentAppHandler := handlers.NewPaymentAppHandler(paymentAppUsecase) // Added
	paymentResolveHandler := handlers.NewPaymentResolveHandler(jweService, complianceService, resolveAuditRepo, paymentRequestUsecase)
	createPaymentHandler := handlers.NewCreatePaymentHandler(createPaymentUsecase)
	partnerQuoteHandler := handlers.NewPartnerQuoteHandler(partnerQuoteUsecase)
	partnerPaymentSessionHandler := handlers.NewPartnerPaymentSessionHandler(partnerPaymentSessionUsecase, complianceService, resolveAuditRepo)
	paymentConfigHandler := handlers.NewPaymentConfigHandler(paymentBridgeRepo, bridgeConfigRepo, feeConfigRepo, chainRepo, tokenRepo)
	onchainAdapterHandler := handlers.NewOnchainAdapterHandler(onchainAdapterUsecase)
	contractConfigAuditHandler := handlers.NewContractConfigAuditHandler(contractConfigAuditUsecase)
	crosschainConfigHandler := handlers.NewCrosschainConfigHandler(crosschainConfigUsecase)
	crosschainPolicyHandler := handlers.NewCrosschainPolicyHandler(routePolicyRepo, stargateConfigRepo, chainRepo)
	routeErrorHandler := handlers.NewRouteErrorHandler(routeErrorUsecase)
	rpcHandler := handlers.NewRpcHandler(chainRepo)
	gasProfilerHandler := handlers.NewGasProfilerHandler(clientFactory) // Added gas profiler

	// Create dual auth middleware
	dualAuthMiddleware := middleware.DualAuthMiddleware(jwtService, apiKeyUsecase, merchantRepo, sessionStore)
	partnerAuthMiddleware := middleware.ApiKeyPartnerMiddleware(apiKeyUsecase, merchantRepo)

	// Create idempotency middleware
	idempotencyMiddleware := middleware.IdempotencyMiddleware()

	// Start background jobs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expiryJob := jobs.NewPaymentRequestExpiryJob(paymentRequestRepo)
	go expiryJob.Start(ctx)
	go webhookJob.Run(ctx)

	// Initialize router
	// Initialize router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggerMiddleware())
	r.Use(idempotencyMiddleware) // Add idempotency middleware

	applyCORSMiddleware(r)
	registerHealthRoute(r)
	registerAPIV1Routes(r, routeDeps{
		authHandler:                    authHandler,
		paymentHandler:                 paymentHandler,
		merchantHandler:                merchantHandler,
		walletHandler:                  walletHandler,
		chainHandler:                   chainHandler,
		tokenHandler:                   tokenHandler,
		smartContractHandler:           smartContractHandler,
		paymentRequestHandler:          paymentRequestHandler,
		webhookHandler:                 webhookHandler,
		adminHandler:                   adminHandler,
		adminMerchantSettlementHandler: adminMerchantSettlementHandler,
		merchantSettlementHandler:      merchantSettlementHandler,
		teamHandler:                    teamHandler,
		apiKeyHandler:                  apiKeyHandler,
		paymentAppHandler:              paymentAppHandler,
		paymentConfigHandler:           paymentConfigHandler,
		onchainAdapterHandler:          onchainAdapterHandler,
		contractConfigAuditHandler:     contractConfigAuditHandler,
		crosschainConfigHandler:        crosschainConfigHandler,
		crosschainPolicyHandler:        crosschainPolicyHandler,
		routeErrorHandler:              routeErrorHandler,
		rpcHandler:                     rpcHandler,
		paymentResolveHandler:          paymentResolveHandler,
		createPaymentHandler:           createPaymentHandler,
		gasProfilerHandler:             gasProfilerHandler, // Added
		partnerQuoteHandler:            partnerQuoteHandler,
		partnerPaymentSessionHandler:   partnerPaymentSessionHandler,
		auditLogRepo:                   auditLogRepo,
		dualAuthMiddleware:             dualAuthMiddleware,
		partnerAuthMiddleware:          partnerAuthMiddleware,
	})

	// Print all registered routes for debugging
	log.Println("📋 Registered Routes:")
	for _, route := range r.Routes() {
		log.Printf("   %s %s", route.Method, route.Path)
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("🛑 Shutting down server...")
		expiryJob.Stop()
		cancel()
	}()

	// Start server
	log.Printf("🚀 Payment-Kita Backend starting on port %s", cfg.Server.Port)
	log.Printf("📚 API: http://localhost:%s/api/v1", cfg.Server.Port)
	log.Printf("❤️ Health: http://localhost:%s/health", cfg.Server.Port)

	if err := runServer(r, cfg.Server.Port); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}
