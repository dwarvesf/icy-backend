package controller

import (
	"github.com/dwarvesf/icy-backend/internal/model"
)

type IController interface {
	// TriggerSwap initiates a swap operation with ICY amount
	TriggerSwap(icyAmount *model.Web3BigInt, satAmount *model.Web3BigInt, btcAddress string) (string, error)

	// ConfirmLatestPrice gets and validates the latest ICY/BTC price
	ConfirmLatestPrice() (*model.Web3BigInt, error)

	// GetProcessedTxByIcyTransactionHash retrieves an onchain ICY transaction by its hash
	GetProcessedTxByIcyTransactionHash(txHash string) (*model.OnchainBtcProcessedTransaction, error)
}
