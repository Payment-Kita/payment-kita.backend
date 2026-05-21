package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	Email         string     `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name          string     `gorm:"type:varchar(100);not null"`
	PasswordHash  string     `gorm:"type:varchar(255);not null"`
	Role          string     `gorm:"type:varchar(50);not null;default:'user'"`
	KYCStatus     string     `gorm:"type:varchar(50);default:'not_started'"`
	KYCVerifiedAt *time.Time `gorm:"type:timestamp"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}
