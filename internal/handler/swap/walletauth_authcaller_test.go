package swap

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/dwarvesf/icy-backend/contracts/icyBtcSwap"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// authMockBaseRPC is a minimal baserpc.IBaseRPC double. Only ICYBalanceOf is
// exercised by authenticateCaller (via requireSufficientICY); it returns a
// balance large enough to clear the anti-Sybil gate so the test focuses on the
// nonce/replay path. Every other method is an unused stub.
type authMockBaseRPC struct {
	balance string
}

func (m *authMockBaseRPC) ICYBalanceOf(string) (*model.Web3BigInt, error) {
	return &model.Web3BigInt{Value: m.balance, Decimal: 18}, nil
}
func (m *authMockBaseRPC) Client() *ethclient.Client          { return nil }
func (m *authMockBaseRPC) GetContractAddress() common.Address { return common.Address{} }
func (m *authMockBaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	return &model.Web3BigInt{Value: "0", Decimal: 18}, nil
}
func (m *authMockBaseRPC) ICYTransferredTo(string, common.Address) (*big.Int, error) {
	return big.NewInt(0), nil
}
func (m *authMockBaseRPC) GetTransactionsByAddress(string, string) ([]model.OnchainIcyTransaction, error) {
	return nil, nil
}
func (m *authMockBaseRPC) Swap(*model.Web3BigInt, string, *model.Web3BigInt) (*types.Transaction, error) {
	return nil, nil
}
func (m *authMockBaseRPC) GenerateSignature(*model.Web3BigInt, string, *model.Web3BigInt, *big.Int, *big.Int) (string, error) {
	return "", nil
}

// newAuthTestHandler builds a handler whose config matches the test EIP-712
// domain (testChainID / testContract) and whose baseRPC reports a large ICY
// balance, so authenticateCaller reaches the nonce/replay logic.
func newAuthTestHandler(t *testing.T) *handler {
	t.Helper()
	cfg := &config.AppConfig{}
	cfg.ApiServer.WalletAuthChainID = testChainID
	cfg.Blockchain.ICYSwapContractAddr = testContract
	return &handler{
		logger:    logger.New(environments.Test),
		appConfig: cfg,
		baseRPC:   &authMockBaseRPC{balance: "1000000000000000000000"},
	}
}

func (m *authMockBaseRPC) FilterSwapEvents(uint64, uint64) ([]*icyBtcSwap.IcyBtcSwapSwap, error) {
	return nil, nil
}
