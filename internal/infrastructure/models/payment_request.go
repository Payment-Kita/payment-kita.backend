package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentRequest struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	MerchantID    uuid.UUID `gorm:"type:uuid;not null;index"`
	ChainID       uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenID       uuid.UUID `gorm:"type:uuid;not null;index"`
	WalletAddress string    `gorm:"column:wallet_address;type:varchar(255);not null"`
	Amount        string    `gorm:"type:decimal(36,18);not null"`
	Decimals      int       `gorm:"not null"`
	Description   string    `gorm:"type:text"`
	Status        string    `gorm:"type:varchar(50);not null;index"`
	ExpiresAt     time.Time `gorm:"not null"`
	TxHash        string    `gorm:"type:varchar(255)"`
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`

	Chain Chain `gorm:"foreignKey:ChainID;references:ID"`
	Token Token `gorm:"foreignKey:TokenID;references:ID"`
}

type BackgroundJob struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	JobType      string    `gorm:"type:varchar(50);not null;index"`
	Payload      string    `gorm:"type:jsonb;not null"`
	Status       string    `gorm:"type:varchar(50);not null;index"`
	Attempts     int       `gorm:"default:0"`
	MaxAttempts  int       `gorm:"not null"`
	ScheduledAt  time.Time `gorm:"index"`
	StartedAt    *time.Time
	CompletedAt  *time.Time
	ErrorMessage string `gorm:"type:text"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
