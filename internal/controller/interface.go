package controller

import "github.com/dwarvesf/icy-backend/internal/model"

type IController interface {
	// TriggerSwap initiates a swap operation with ICY amount
	TriggerSwap(icyAmount *model.Web3BigInt, btcAmount *model.Web3BigInt, btcAddress string) error

	// ConfirmLatestPrice gets and validates the latest ICY/BTC price
	ConfirmLatestPrice() (*model.Web3BigInt, error)

	// TriggerSendBTC initiates BTC transfer if tx fee is under threshold
	TriggerSendBTC(address string, amount *model.Web3BigInt) error

	// WatchSwapEvents monitors and handles swap contract events
	WatchSwapEvents() error
}
