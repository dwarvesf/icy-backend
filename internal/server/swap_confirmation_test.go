package server

import (
	"math/big"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// SG-11 item 1: the Base-side confirmation gate. A swap payout is created only
// once the triggering Base swap event is buried >= N confirmations deep, so a
// shallow reorg cannot leave the treasury having paid BTC for a swap that no
// longer exists (this also closes SG-12's zero-confirmation gap). Tested through
// the exported telemetry.ProcessConfirmedSwap and telemetry.EnoughConfirmations
// because internal/telemetry's own test package does not build (pre-existing
// breakage in btc_multi_endpoint_test.go, unrelated to SG-11).

// EnoughConfirmations is the pure depth predicate. Inclusion block counts as the
// first confirmation, so with N=6 an event needs to be in a block <= tip-5.
func TestEnoughConfirmations_Boundary(t *testing.T) {
	const n = 6
	cases := []struct {
		name        string
		eventBlock  uint64
		latestBlock uint64
		want        bool
	}{
		{"tip block = 1 conf, below N", 100, 100, false},
		{"5 confs, one short of N", 100, 104, false},
		{"exactly N confs", 100, 105, true},
		{"deeper than N", 100, 200, true},
		{"latest behind event (reorg shortened chain), fail-closed", 100, 99, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := telemetry.EnoughConfirmations(c.eventBlock, c.latestBlock, n); got != c.want {
				t.Fatalf("EnoughConfirmations(%d, %d, %d) = %v, want %v",
					c.eventBlock, c.latestBlock, n, got, c.want)
			}
		})
	}
}

// N == 0 disables the gate (action as soon as mined) so the knob can be turned
// off without code changes.
func TestEnoughConfirmations_ZeroDisablesGate(t *testing.T) {
	if !telemetry.EnoughConfirmations(100, 100, 0) {
		t.Fatal("minConfirmations=0 must disable the gate (always enough)")
	}
}

// newSwapTestDB migrates BOTH the swap and the payout tables so
// ProcessConfirmedSwap can store the swap row and its payout.
func newSwapTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.OnchainBtcProcessedTransaction{}, &model.OnchainIcySwapTransaction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, tbl := range []string{"onchain_btc_processed_transactions", "onchain_icy_swap_transactions"} {
		if err := db.Exec("DELETE FROM " + tbl).Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	return db
}

func newTelemetryWithConfirmations(t *testing.T, db *gorm.DB, base *mockBaseRpc, minSwapConf int64) *telemetry.Telemetry {
	t.Helper()
	cfg := &config.AppConfig{}
	cfg.Blockchain.MinSwapConfirmations = minSwapConf
	return telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), nil, base, nil)
}

func swapAtBlock(hash string, block uint64, icyAmount, btcAmount string) *model.OnchainIcySwapTransaction {
	return &model.OnchainIcySwapTransaction{
		TransactionHash: hash,
		BlockNumber:     block,
		IcyAmount:       icyAmount,
		BtcAddress:      "bc1qexampleaddr",
		BtcAmount:       btcAmount,
	}
}

func countSwapRows(t *testing.T, db *gorm.DB, hash string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.OnchainIcySwapTransaction{}).
		Where("transaction_hash = ?", hash).Count(&n).Error; err != nil {
		t.Fatalf("count swap rows: %v", err)
	}
	return n
}

// MONEY PROOF (below threshold): a swap event with fewer than N confirmations is
// NOT actioned. No payout row is created (and no swap row is recorded), so a
// reorg that unwinds it leaves nothing behind. Then, once the same event crosses
// N confirmations, it IS actioned exactly once.
func TestProcessConfirmedSwap_UnderThenOverThreshold(t *testing.T) {
	db := newSwapTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xconf": big.NewInt(1000)},
	}
	tel := newTelemetryWithConfirmations(t, db, base, 6)
	sw := swapAtBlock("0xconf", 100, "1000", "5000")

	// Under-confirmed: event at block 100, tip at 104 -> only 5 confirmations.
	actioned, err := tel.ProcessConfirmedSwap(db, sw, 104)
	if err != nil {
		t.Fatalf("under-confirmed call: %v", err)
	}
	if actioned {
		t.Fatal("under-confirmed swap reported as actioned, want false")
	}
	if n := countPayouts(t, db, "0xconf"); n != 0 {
		t.Fatalf("under-confirmed swap created %d payout rows, want 0", n)
	}
	if n := countSwapRows(t, db, "0xconf"); n != 0 {
		t.Fatalf("under-confirmed swap recorded %d swap rows, want 0", n)
	}

	// Now buried >= 6 deep: event at block 100, tip at 105 -> 6 confirmations.
	actioned, err = tel.ProcessConfirmedSwap(db, sw, 105)
	if err != nil {
		t.Fatalf("confirmed call: %v", err)
	}
	if !actioned {
		t.Fatal("confirmed swap reported as NOT actioned, want true")
	}
	if n := countPayouts(t, db, "0xconf"); n != 1 {
		t.Fatalf("confirmed swap created %d payout rows, want 1", n)
	}
	if n := countSwapRows(t, db, "0xconf"); n != 1 {
		t.Fatalf("confirmed swap recorded %d swap rows, want 1", n)
	}
}

// DEDUP still holds under the gate (respects SG-12): actioning the same confirmed
// swap twice must not create a second payout or a second swap row.
func TestProcessConfirmedSwap_ConfirmedTwice_NoDoubleAction(t *testing.T) {
	db := newSwapTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xdup2": big.NewInt(1000)},
	}
	tel := newTelemetryWithConfirmations(t, db, base, 6)
	sw := swapAtBlock("0xdup2", 100, "1000", "5000")

	if _, err := tel.ProcessConfirmedSwap(db, sw, 200); err != nil {
		t.Fatalf("action #1: %v", err)
	}
	if _, err := tel.ProcessConfirmedSwap(db, sw, 200); err != nil {
		t.Fatalf("action #2: %v", err)
	}
	if n := countPayouts(t, db, "0xdup2"); n != 1 {
		t.Fatalf("re-actioned confirmed swap minted %d payouts, want 1", n)
	}
	if n := countSwapRows(t, db, "0xdup2"); n != 1 {
		t.Fatalf("re-actioned confirmed swap recorded %d swap rows, want 1", n)
	}
}
