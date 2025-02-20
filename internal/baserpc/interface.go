package baserpc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IBaseRPC interface {
	Client() *ethclient.Client
	ICYBalanceOf(address string) (*model.Web3BigInt, error)
	ICYTotalSupply() (*model.Web3BigInt, error)
	GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error)
	Swap(
		icyAmount *model.Web3BigInt,
		btcAddress string,
		btcAmount *model.Web3BigInt,
	) (*types.Transaction, error)
	GenerateSignature(
		icyAmount *model.Web3BigInt,
		btcAddress string,
		btcAmount *model.Web3BigInt,
		nonce *big.Int,
		deadline *big.Int,
	) (string, error)
}
