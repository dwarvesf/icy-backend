package model

import (
	"time"

	"gorm.io/gorm"
)

type SwapRequest struct {
	gorm.Model
	ICYAmount   string     `gorm:"column:icy_amount;type:varchar(255);not null"`
	BTCAddress  string     `gorm:"column:btc_address;type:varchar(255);not null"`
	IcyTx       string     `gorm:"column:icy_tx;type:varchar(255);not null;uniqueIndex"`
	Status      string     `gorm:"column:status;type:varchar(50);default:'pending'"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
}

func (SwapRequest) TableName() string {
	return "swap_requests"
}
