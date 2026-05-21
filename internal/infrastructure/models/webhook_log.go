package models

import (
	"time"

	"github.com/google/uuid"
)

type WebhookLog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v7()"`
	MerchantID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	PaymentID      uuid.UUID  `gorm:"type:uuid;not null"`
	EventType      string     `gorm:"type:varchar(50);not null"`
	Payload        string     `gorm:"type:jsonb;not null"`
	DeliveryStatus string     `gorm:"type:webhook_delivery_status;default:pending;index"`
	HttpStatus     int        `gorm:"column:http_status"`
	ResponseBody   string     `gorm:"type:text"`
	RetryCount     int        `gorm:"default:0"`
	NextRetryAt    *time.Time `gorm:"index"`
	LastAttemptAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	Merchant Merchant `gorm:"foreignKey:MerchantID"`
	Payment  Payment  `gorm:"foreignKey:PaymentID"`
}

func (WebhookLog) TableName() string {
	return "webhook_logs"
}
