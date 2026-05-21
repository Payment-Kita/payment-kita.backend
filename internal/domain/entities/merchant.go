package entities

import (
	"time"

	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
)

// MerchantType represents merchant types
type MerchantType string

const (
	MerchantTypePartner   MerchantType = "PARTNER"
	MerchantTypeCorporate MerchantType = "CORPORATE"
	MerchantTypeUMKM      MerchantType = "UMKM"
	MerchantTypeRetail    MerchantType = "RETAIL"
	MerchantTypeServices  MerchantType = "SERVICES"
	MerchantTypeDigital   MerchantType = "DIGITAL"
	MerchantTypeOther     MerchantType = "OTHER"
)

// MerchantStatus represents merchant verification status
type MerchantStatus string

const (
	MerchantStatusPending   MerchantStatus = "PENDING"
	MerchantStatusActive    MerchantStatus = "ACTIVE"
	MerchantStatusSuspended MerchantStatus = "SUSPENDED"
	MerchantStatusRejected  MerchantStatus = "REJECTED"
)

// Merchant represents a merchant entity
type Merchant struct {
	ID                 uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v7()"`
	UserID             uuid.UUID      `json:"userId"`
	BusinessName       string         `json:"businessName"`
	BusinessEmail      string         `json:"businessEmail"`
	MerchantType       MerchantType   `json:"merchantType"`
	Status             MerchantStatus `json:"status"`
	BusinessWebsite    string         `json:"businessWebsite,omitempty"`
	BusinessDescription string         `json:"businessDescription,omitempty"`
	TaxID              null.String    `json:"taxId,omitempty"`
	BusinessAddress    null.String    `json:"businessAddress,omitempty"`
	Documents          null.JSON      `json:"documents,omitempty"`
	FeeDiscountPercent string         `json:"feeDiscountPercent" gorm:"type:decimal(5,2)"` // Changed to string
	CallbackURL        string         `json:"callbackUrl,omitempty"`
	WebhookSecret      string         `json:"webhookSecret,omitempty"`
	WebhookIsActive    bool           `json:"webhookIsActive"`
	SupportEmail       string         `json:"supportEmail,omitempty"`
	LogoURL            string         `json:"logoUrl,omitempty"`
	WebhookMetadata    null.JSON      `json:"webhookMetadata,omitempty"`
	Metadata           null.JSON      `json:"metadata,omitempty"`
	VerifiedAt         *time.Time     `json:"verifiedAt,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          *time.Time     `json:"-"`
}

// MerchantApplyInput represents input for merchant application
type MerchantApplyInput struct {
	MerchantType    MerchantType `json:"merchantType" binding:"required"`
	BusinessName    string       `json:"businessName" binding:"required,min=2,max=255"`
	BusinessEmail   string       `json:"businessEmail" binding:"required,email"`
	TaxID           string       `json:"taxId,omitempty"`
	BusinessAddress string       `json:"businessAddress,omitempty"`
	Documents       interface{}  `json:"documents,omitempty"`
}

// MerchantStatusResponse represents merchant status response
type MerchantStatusResponse struct {
	MerchantID      uuid.UUID      `json:"merchantId"`
	Status          MerchantStatus `json:"status"`
	MerchantType    MerchantType   `json:"merchantType"`
	BusinessName    string         `json:"businessName"`
	Message         string         `json:"message,omitempty"`
	RejectionReason null.String    `json:"rejectionReason,omitempty"`
	SubmittedAt     time.Time      `json:"submittedAt"`
	ReviewedAt      *time.Time     `json:"reviewedAt,omitempty"`
}
