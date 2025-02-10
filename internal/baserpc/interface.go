package baserpc

import (
	"math/big"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type IBaseRPC interface {
	Client() *ethclient.Client
	ICYBalanceOf(address string) (*model.Web3BigInt, error)
	ICYTotalSupply() (*model.Web3BigInt, error)
	GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error)
	Swap(
		icyAmount *big.Int, 
		btcAddress string, 
		btcAmount *big.Int, 
		nonce *big.Int, 
		deadline *big.Int, 
		signature []byte,
	) (*types.Transaction, error)
}
