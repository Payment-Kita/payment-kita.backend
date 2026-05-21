package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"gorm.io/gorm"
	"payment-kita.backend/internal/domain/entities"
	domainerrors "payment-kita.backend/internal/domain/errors"
	"payment-kita.backend/internal/domain/repositories"
	"payment-kita.backend/internal/infrastructure/models"
	"payment-kita.backend/pkg/utils"
)

// TokenRepository implements token data operations
type TokenRepository struct {
	db        *gorm.DB
	chainRepo repositories.ChainRepository
}

// NewTokenRepository creates a new token repository
func NewTokenRepository(db *gorm.DB, chainRepo repositories.ChainRepository) *TokenRepository {
	return &TokenRepository{
		db:        db,
		chainRepo: chainRepo,
	}
}

// GetByID gets a token by ID
func (r *TokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.Token, error) {
	var m models.Token
	// Preload Chain
	if err := r.db.WithContext(ctx).Preload("Chain").Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, err
	}
	return r.toEntity(&m), nil
}

// GetBySymbol gets a token by symbol and chain ID
func (r *TokenRepository) GetBySymbol(ctx context.Context, symbol string, chainID uuid.UUID) (*entities.Token, error) {
	var m models.Token
	if err := r.db.WithContext(ctx).Preload("Chain").Where("symbol = ? AND chain_id = ?", symbol, chainID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, err
	}
	return r.toEntity(&m), nil
}

// GetByAddress gets a token by contract address and chain ID
func (r *TokenRepository) GetByAddress(ctx context.Context, address string, chainID uuid.UUID) (*entities.Token, error) {
	var m models.Token
	normalizedAddress := strings.TrimSpace(strings.ToLower(address))
	if err := r.db.WithContext(ctx).
		Preload("Chain").
		Where("LOWER(address) = ? AND chain_id = ?", normalizedAddress, chainID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, err
	}
	return r.toEntity(&m), nil
}

// GetAll gets all tokens
func (r *TokenRepository) GetAll(ctx context.Context) ([]*entities.Token, error) {
	var ms []models.Token
	if err := r.db.WithContext(ctx).Preload("Chain").Order("symbol").Find(&ms).Error; err != nil {
		return nil, err
	}

	var tokens []*entities.Token
	for _, m := range ms {
		model := m
		tokens = append(tokens, r.toEntity(&model))
	}
	return tokens, nil
}

// GetStablecoins gets only stablecoin tokens
func (r *TokenRepository) GetStablecoins(ctx context.Context) ([]*entities.Token, error) {
	var ms []models.Token
	if err := r.db.WithContext(ctx).Preload("Chain").Where("is_stablecoin = ?", true).Order("symbol").Find(&ms).Error; err != nil {
		return nil, err
	}

	var tokens []*entities.Token
	for _, m := range ms {
		model := m
		tokens = append(tokens, r.toEntity(&model))
	}
	return tokens, nil
}

// GetNative gets the native token for a chain
func (r *TokenRepository) GetNative(ctx context.Context, chainID uuid.UUID) (*entities.Token, error) {
	var m models.Token
	if err := r.db.WithContext(ctx).
		Where("chain_id = ? AND is_native = ?", chainID, true).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, err
	}
	return r.toEntity(&m), nil
}

// GetTokensByChain gets tokens supported on a chain
func (r *TokenRepository) GetTokensByChain(ctx context.Context, chainID uuid.UUID, pagination utils.PaginationParams) ([]*entities.Token, int64, error) {
	var ms []models.Token
	var totalCount int64

	// Count unique contract addresses per chain
	if err := r.db.WithContext(ctx).Model(&models.Token{}).
		Where("chain_id = ? AND is_active = ? AND deleted_at IS NULL", chainID, true).
		Distinct("address").
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	// Use a subquery to get the latest token for each contract address
	// This prevents duplicates when multiple tokens have the same contract address
	query := r.db.WithContext(ctx).Table("tokens t1").
		Joins(`JOIN (
			SELECT address, MAX(updated_at) as max_updated_at
			FROM tokens
			WHERE chain_id = ? AND is_active = ? AND deleted_at IS NULL
			GROUP BY address
		) t2 ON t1.address = t2.address AND t1.updated_at = t2.max_updated_at`, chainID, true).
		Preload("Chain").
		Order("t1.symbol")

	if pagination.Limit > 0 {
		query = query.Limit(pagination.Limit).Offset(pagination.CalculateOffset())
	}

	if err := query.Find(&ms).Error; err != nil {
		return nil, 0, err
	}

	var tokens []*entities.Token
	for _, m := range ms {
		model := m
		tokens = append(tokens, r.toEntity(&model))
	}
	return tokens, totalCount, nil
}

// GetAllTokens gets all tokens with filters
func (r *TokenRepository) GetAllTokens(ctx context.Context, chainID *uuid.UUID, search *string, pagination utils.PaginationParams) ([]*entities.Token, int64, error) {
	var ms []models.Token
	var totalCount int64

	// Build base conditions
	conditions := "deleted_at IS NULL"
	args := []interface{}{}

	if chainID != nil {
		conditions += " AND chain_id = ?"
		args = append(args, *chainID)
	}

	// Count unique contract addresses (with optional chain filter)
	if err := r.db.WithContext(ctx).Model(&models.Token{}).
		Where(conditions, args...).
		Distinct("address").
		Count(&totalCount).Error; err != nil {
		return nil, 0, err
	}

	// Build search conditions
	searchConditions := conditions
	searchArgs := args

	if search != nil && *search != "" {
		term := "%" + *search + "%"
		searchConditions += " AND (symbol ILIKE ? OR name ILIKE ? OR address ILIKE ?)"
		searchArgs = append(searchArgs, term, term, term)
	}

	// Use a subquery to get the latest token for each contract address
	// This prevents duplicates when multiple tokens have the same contract address
	query := r.db.WithContext(ctx).Table("tokens t1").
		Joins(`JOIN (
			SELECT address, MAX(updated_at) as max_updated_at
			FROM tokens
			WHERE `+conditions+`
			GROUP BY address
		) t2 ON t1.address = t2.address AND t1.updated_at = t2.max_updated_at`, args...)

	// Apply search filter to the main query
	if search != nil && *search != "" {
		term := "%" + *search + "%"
		query = query.Where("t1.symbol ILIKE ? OR t1.name ILIKE ? OR t1.address ILIKE ?", term, term, term)
	}

	query = query.Preload("Chain").Order("t1.symbol")

	if pagination.Limit > 0 {
		query = query.Limit(pagination.Limit).Offset(pagination.CalculateOffset())
	}

	if err := query.Find(&ms).Error; err != nil {
		return nil, 0, err
	}

	var tokens []*entities.Token
	for _, m := range ms {
		model := m
		tokens = append(tokens, r.toEntity(&model))
	}
	return tokens, totalCount, nil
}

func (r *TokenRepository) toEntity(m *models.Token) *entities.Token {
	e := &entities.Token{
		ID:              m.ID,
		ChainUUID:       m.ChainID, // Changed ChainID to ChainUUID
		Symbol:          m.Symbol,
		Name:            m.Name,
		Decimals:        m.Decimals,
		LogoURL:         m.LogoURL,
		ContractAddress: m.ContractAddress,
		Type:            entities.TokenType(m.Type),
		IsActive:        m.IsActive,
		IsNative:        m.IsNative,
		IsStablecoin:    m.IsStablecoin,
		MinAmount:       m.MinAmount,
		MaxAmount:       null.StringFromPtr(m.MaxAmount), // Added MaxAmount
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
		DeletedAt:       &m.DeletedAt.Time, // Added DeletedAt
	}

	// Populating BlockchainID from Chain if available
	// Populating BlockchainID from Chain if available
	if m.Chain.ID != uuid.Nil {
		// Map directly from preloaded model to avoid N+1
		e.Chain = &entities.Chain{
			ID:             m.Chain.ID,
			ChainID:        m.Chain.NetworkID,
			Name:           m.Chain.Name,
			Type:           entities.ChainType(strings.ToUpper(m.Chain.ChainType)),
			RPCURL:         m.Chain.RPCURL,
			ExplorerURL:    m.Chain.ExplorerURL,
			CurrencySymbol: m.Chain.Symbol,
			ImageURL:       m.Chain.LogoURL,
			IsActive:       m.Chain.IsActive,
			CreatedAt:      m.Chain.CreatedAt,
			UpdatedAt:      m.Chain.UpdatedAt,
		}
		e.BlockchainID = e.Chain.ChainID
	} else {
		// Try to find blockchainId by matching ChainUUID if needed,
		// but usually it should be preloaded.
		e.BlockchainID = "" // Or fallback logic
	}

	return e
}

func (r *TokenRepository) toModel(token *entities.Token) *models.Token {
	return &models.Token{
		ID:              token.ID,
		ChainID:         token.ChainUUID,
		Symbol:          token.Symbol,
		Name:            token.Name,
		Decimals:        token.Decimals,
		ContractAddress: token.ContractAddress,
		Type:            string(token.Type),
		LogoURL:         token.LogoURL,
		IsActive:        token.IsActive,
		IsNative:        token.IsNative,
		IsStablecoin:    token.IsStablecoin,
		MinAmount:       token.MinAmount,
		MaxAmount:       token.MaxAmount.Ptr(),
		CreatedAt:       token.CreatedAt,
		UpdatedAt:       token.UpdatedAt,
	}
}

// Create creates a new token
func (r *TokenRepository) Create(ctx context.Context, token *entities.Token) error {
	m := r.toModel(token)

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	return nil
}

// Update updates an existing token
func (r *TokenRepository) Update(ctx context.Context, token *entities.Token) error {
	m := r.toModel(token)
	return r.db.WithContext(ctx).Save(m).Error
}

// SoftDelete soft deletes a token
func (r *TokenRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Token{}, "id = ?", id).Error
}
