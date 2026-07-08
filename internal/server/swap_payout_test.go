package server

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// mockBaseRpc is a test double for baserpc.IBaseRPC. Only GetContractAddress
// (the ICY treasury) and ICYTransferredTo (the verified on-chain deposit) are
// exercised by CreateBtcPayoutForSwap; every other method is a stub.
type mockBaseRpc struct {
	treasury   common.Address
	deposited  map[string]*big.Int // swap tx hash -> ICY transferred to treasury
	depositErr error
}

func (m *mockBaseRpc) GetContractAddress() common.Address { return m.treasury }

func (m *mockBaseRpc) ICYTransferredTo(txHash string, to common.Address) (*big.Int, error) {
	if m.depositErr != nil {
		return nil, m.depositErr
	}
	if to != m.treasury {
		return big.NewInt(0), nil
	}
	if v, ok := m.deposited[txHash]; ok {
		return v, nil
	}
	return big.NewInt(0), nil
}

func (m *mockBaseRpc) Client() *ethclient.Client { return nil }
func (m *mockBaseRpc) ICYBalanceOf(string) (*model.Web3BigInt, error) {
	return &model.Web3BigInt{Value: "0", Decimal: 18}, nil
}
func (m *mockBaseRpc) ICYTotalSupply() (*model.Web3BigInt, error) {
	return &model.Web3BigInt{Value: "0", Decimal: 18}, nil
}
func (m *mockBaseRpc) GetTransactionsByAddress(string, string) ([]model.OnchainIcyTransaction, error) {
	return nil, nil
}
func (m *mockBaseRpc) Swap(*model.Web3BigInt, string, *model.Web3BigInt) (*types.Transaction, error) {
	return nil, nil
}
func (m *mockBaseRpc) GenerateSignature(*model.Web3BigInt, string, *model.Web3BigInt, *big.Int, *big.Int) (string, error) {
	return "", nil
}

// treasuryAddr is the fixed ICY swap-contract address used across these tests.
var treasuryAddr = common.HexToAddress("0xC0ffee0000000000000000000000000000000000")

func newTelemetryWithBase(t *testing.T, db *gorm.DB, base *mockBaseRpc) *telemetry.Telemetry {
	t.Helper()
	return telemetry.New(db, store.New(db), &config.AppConfig{}, logger.New(environments.Test), nil, base, nil)
}

func swapEvent(hash, icyAmount, btcAmount string) *model.OnchainIcySwapTransaction {
	return &model.OnchainIcySwapTransaction{
		TransactionHash: hash,
		IcyAmount:       icyAmount,
		BtcAddress:      "bc1qexampleaddr",
		BtcAmount:       btcAmount,
	}
}

func countPayouts(t *testing.T, db *gorm.DB, swapHash string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.OnchainBtcProcessedTransaction{}).
		Where("swap_transaction_hash = ?", swapHash).Count(&n).Error; err != nil {
		t.Fatalf("count payouts: %v", err)
	}
	return n
}

// Happy path: a swap whose ICY deposit meets the required amount produces exactly
// one pending payout row, with the dedup key populated on both columns.
func TestCreateBtcPayout_VerifiedDeposit_CreatesOne(t *testing.T) {
	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xswapok": big.NewInt(1000)},
	}
	tel := newTelemetryWithBase(t, db, base)
	sw := swapEvent("0xswapok", "1000", "5000")

	if err := tel.CreateBtcPayoutForSwap(db, sw); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := countPayouts(t, db, "0xswapok"); n != 1 {
		t.Fatalf("payout rows = %d, want 1", n)
	}

	var row model.OnchainBtcProcessedTransaction
	if err := db.First(&row, "swap_transaction_hash = ?", "0xswapok").Error; err != nil {
		t.Fatalf("read payout: %v", err)
	}
	if row.Status != model.BtcProcessingStatusPending {
		t.Fatalf("status = %q, want pending", row.Status)
	}
	if row.IcyTransactionHash == nil || *row.IcyTransactionHash != "0xswapok" {
		t.Fatalf("icy_transaction_hash = %v, want 0xswapok (dedup column must be populated)", row.IcyTransactionHash)
	}
}

// A deposit at least the required amount (overpayment) is accepted.
func TestCreateBtcPayout_OverDeposit_Accepted(t *testing.T) {
	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xover": big.NewInt(1500)},
	}
	tel := newTelemetryWithBase(t, db, base)

	if err := tel.CreateBtcPayoutForSwap(db, swapEvent("0xover", "1000", "5000")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := countPayouts(t, db, "0xover"); n != 1 {
		t.Fatalf("payout rows = %d, want 1", n)
	}
}

// DEDUP money-proof: indexing the SAME swap tx twice (reorg re-emit / re-scan)
// must create at most one BTC payout row. The second call is a no-op.
func TestCreateBtcPayout_DuplicateSwap_NoSecondPayout(t *testing.T) {
	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xdup": big.NewInt(1000)},
	}
	tel := newTelemetryWithBase(t, db, base)
	sw := swapEvent("0xdup", "1000", "5000")

	if err := tel.CreateBtcPayoutForSwap(db, sw); err != nil {
		t.Fatalf("create #1: %v", err)
	}
	if err := tel.CreateBtcPayoutForSwap(db, sw); err != nil {
		t.Fatalf("create #2: %v", err)
	}
	if n := countPayouts(t, db, "0xdup"); n != 1 {
		t.Fatalf("duplicate swap minted %d payouts, want 1 (no second BTC payout)", n)
	}
}

// VERIFY money-proof (negative control): a SHORT ICY deposit (500 < required
// 1000) is rejected. No payout row is created, so no BTC can ever leave for it.
func TestCreateBtcPayout_ShortDeposit_Rejected(t *testing.T) {
	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xshort": big.NewInt(500)},
	}
	tel := newTelemetryWithBase(t, db, base)

	if err := tel.CreateBtcPayoutForSwap(db, swapEvent("0xshort", "1000", "5000")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := countPayouts(t, db, "0xshort"); n != 0 {
		t.Fatalf("short deposit created %d payouts, want 0", n)
	}
}

// VERIFY money-proof: an ABSENT ICY deposit (nothing transferred to the treasury
// in that tx) is rejected. No payout row.
func TestCreateBtcPayout_AbsentDeposit_Rejected(t *testing.T) {
	db := newTestDB(t)
	base := &mockBaseRpc{treasury: treasuryAddr, deposited: map[string]*big.Int{}} // returns 0
	tel := newTelemetryWithBase(t, db, base)

	if err := tel.CreateBtcPayoutForSwap(db, swapEvent("0xabsent", "1000", "5000")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := countPayouts(t, db, "0xabsent"); n != 0 {
		t.Fatalf("absent deposit created %d payouts, want 0", n)
	}
}
