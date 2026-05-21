package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ApiKey struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Name            string    `gorm:"type:varchar(100);not null"`
	KeyPrefix       string    `gorm:"type:varchar(20);not null"`
	KeyHash         string    `gorm:"type:varchar(64);uniqueIndex;not null"` // SHA256 of key
	SecretEncrypted string    `gorm:"type:text;not null"`                    // AES-256-GCM
	SecretMasked    string    `gorm:"type:varchar(20);not null"`             // "****abcd"
	Permissions     string    `gorm:"type:text;not null"`                    // JSON string
	IsActive        bool      `gorm:"default:true;not null"`
	LastUsedAt      *time.Time
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
	User            User           `gorm:"foreignKey:UserID"`
}
