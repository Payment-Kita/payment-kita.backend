package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Wallet struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	UserID     *uuid.UUID `gorm:"type:uuid;index"`          // Nullable
	MerchantID *uuid.UUID `gorm:"type:uuid;index"`          // Nullable
	ChainID    uuid.UUID  `gorm:"type:uuid;not null;index"` // FK to chains.id
	Address    string     `gorm:"type:varchar(255);not null;index"`
	IsPrimary  bool       `gorm:"default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`

	// Relations
	Chain Chain `gorm:"foreignKey:ChainID;references:ID"`
}
