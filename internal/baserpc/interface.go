package baserpc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IBaseRPC interface {
	Client() *ethclient.Client
	GetContractAddress() common.Address
	ICYBalanceOf(address string) (*model.Web3BigInt, error)
	ICYTotalSupply() (*model.Web3BigInt, error)
	// ICYTransferredTo returns the total configured-ICY (ERC20) amount transferred
	// to `to` within transaction `txHash`, summed across every ICY Transfer log in
	// that tx's receipt (0 if none). Used to verify a swap's ICY deposit actually
	// landed in the treasury before any BTC payout row is created.
	ICYTransferredTo(txHash string, to common.Address) (*big.Int, error)
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
