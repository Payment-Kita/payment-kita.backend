package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Chain struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	NetworkID string    `gorm:"type:text;not null;uniqueIndex;column:chain_id"` // Mapped to chain_id column
	// Namespace      string    `gorm:"-"` // Deprecated: Derived from Type
	Name              string `gorm:"type:varchar(100);not null"`
	ChainType         string `gorm:"type:varchar(50);not null;default:'EVM';column:type"`
	RPCURL            string `gorm:"type:text;column:rpc_url"`
	ExplorerURL       string `gorm:"type:text"`
	Symbol            string `gorm:"type:varchar(20);column:currency_symbol"`
	LogoURL           string `gorm:"type:text;column:image_url"`
	IsActive          bool   `gorm:"default:true"`
	StateMachineID    string `gorm:"type:varchar(100)"`
	CCIPChainSelector string `gorm:"type:varchar(255);column:ccip_chain_selector"`
	StargateEID      int    `gorm:"type:integer;column:stargate_eid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`

	// Relations
	RPCs []ChainRPC `gorm:"foreignKey:ChainID;references:ID"`
}

type ChainRPC struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	ChainID     uuid.UUID `gorm:"type:uuid;not null;index"`
	URL         string    `gorm:"type:text;not null"`
	Priority    int       `gorm:"default:0"`
	IsActive    bool      `gorm:"default:true;index"`
	LastErrorAt *time.Time
	ErrorCount  int `gorm:"default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`

	// Relations
	Chain Chain `gorm:"foreignKey:ChainID;references:ID"`
}
