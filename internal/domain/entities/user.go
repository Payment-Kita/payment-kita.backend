package entities

import (
	"time"

	"github.com/google/uuid"
)

// UserRole represents user roles
type UserRole string

const (
	UserRoleAdmin    UserRole = "ADMIN"
	UserRoleSubAdmin UserRole = "SUB_ADMIN"
	UserRolePartner  UserRole = "PARTNER"
	UserRoleUser     UserRole = "USER"
)

// KYCStatus represents KYC verification status
type KYCStatus string

const (
	KYCNotStarted       KYCStatus = "NOT_STARTED"
	KYCIDCardVerified   KYCStatus = "ID_CARD_VERIFIED"
	KYCFaceVerified     KYCStatus = "FACE_VERIFIED"
	KYCLivenessVerified KYCStatus = "LIVENESS_VERIFIED"
	KYCFullyVerified    KYCStatus = "FULLY_VERIFIED"
)

// User represents a user entity
type User struct {
	ID            uuid.UUID  `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v7()"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	PasswordHash  string     `json:"-"`
	Role          UserRole   `json:"role"`
	KYCStatus     KYCStatus  `json:"kycStatus"`
	KYCVerifiedAt *time.Time `json:"kycVerifiedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	DeletedAt     *time.Time `json:"-"`
}

// CreateUserInput represents input for creating a user
type CreateUserInput struct {
	Email           string `json:"email" binding:"required,email"`
	Name            string `json:"name" binding:"required,min=2,max=100"`
	Password        string `json:"password" binding:"required,min=8"`
	WalletAddress   string `json:"walletAddress" binding:"required"`
	WalletChainID   string `json:"walletChainId" binding:"required"` // The NetworkID (e.g. "84532")
	WalletSignature string `json:"walletSignature" binding:"required"`

	// Merchant fields (unified registration)
	IsMerchant          bool   `json:"isMerchant"`
	BusinessName        string `json:"businessName"`
	BusinessCategory    string `json:"businessCategory"`
	BusinessWebsite     string `json:"businessWebsite"`
	BusinessDescription string `json:"businessDescription"`
	MerchantType        string `json:"merchantType"` // Deprecated in favor of BusinessCategory but kept for compatibility
}

// LoginInput represents input for user login
type LoginInput struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
	UseSession bool   `json:"useSession"` // If true, store tokens in Redis and return SessionID
}

// AuthResponse represents authentication response
type AuthResponse struct {
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	User         *User  `json:"user"`
}

// ChangePasswordInput represents input for changing user password.
type ChangePasswordInput struct {
	CurrentPassword string `json:"currentPassword" binding:"required,min=8"`
	NewPassword     string `json:"newPassword" binding:"required,min=8"`
}
