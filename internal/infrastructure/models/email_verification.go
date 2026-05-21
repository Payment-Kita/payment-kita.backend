package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailVerification struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Token      string    `gorm:"type:varchar(255);not null;index"`
	ExpiresAt  time.Time `gorm:"not null"`
	VerifiedAt *time.Time
	CreatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`

	// Associations
	User User `gorm:"foreignKey:UserID"`
}
