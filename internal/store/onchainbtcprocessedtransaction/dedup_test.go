package onchainbtcprocessedtransaction_test

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
)

// applySwapHashUniqueIndex replicates migration 0013's partial UNIQUE index on
// swap_transaction_hash into the test DB (AutoMigrate does not build raw-SQL
// indexes). This is the DB backstop that makes a second payout row for the same
// swap event impossible even if an application-level guard were bypassed.
func applySwapHashUniqueIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uniq_btc_processed_swap_transaction_hash
		ON onchain_btc_processed_transactions (swap_transaction_hash)
		WHERE swap_transaction_hash IS NOT NULL AND swap_transaction_hash <> ''`).Error
	if err != nil {
		t.Fatalf("apply unique index: %v", err)
	}
}

func seedPayoutForSwap(t *testing.T, db *gorm.DB, swapHash string) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		SwapTransactionHash: swapHash,
		BTCAddress:          "bc1qexampleaddr",
		Subtotal:            "1000",
		ServiceFee:          "100",
		Status:              model.BtcProcessingStatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed payout: %v", err)
	}
	return row.ID
}

// Dedup key is populated + queryable: GetBySwapTransactionHash finds the payout
// row for a swap tx hash that was actually written, and reports ErrRecordNotFound
// for one that was not. This is the lookup the indexer uses to skip a re-indexed
// / reorg-re-emitted swap before minting a second payout.
func TestGetBySwapTransactionHash_FindsAndMisses(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	const swapHash = "0xswapabc"

	if _, err := s.GetBySwapTransactionHash(db, swapHash); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("before seed: got err %v, want ErrRecordNotFound", err)
	}

	id := seedPayoutForSwap(t, db, swapHash)

	got, err := s.GetBySwapTransactionHash(db, swapHash)
	if err != nil {
		t.Fatalf("after seed: %v", err)
	}
	if got.ID != id {
		t.Fatalf("found id %d, want %d", got.ID, id)
	}
}

// DB backstop (migration 0013): a second payout row with the SAME
// swap_transaction_hash is rejected by the partial unique index. Even if two
// indexer runs raced past the application guard, the database makes a duplicate
// BTC payout row impossible. The negative control is the wrong-value case below:
// a DIFFERENT swap hash inserts cleanly, proving the index blocks only the dup.
func TestSwapHashUniqueIndex_RejectsDuplicatePayout(t *testing.T) {
	db := newTestDB(t)
	applySwapHashUniqueIndex(t, db)
	const swapHash = "0xswapdup"

	seedPayoutForSwap(t, db, swapHash) // first row: ok

	// Second row, same swap hash: must be rejected by the unique index.
	dup := &model.OnchainBtcProcessedTransaction{
		SwapTransactionHash: swapHash,
		BTCAddress:          "bc1qother",
		Subtotal:            "2000",
		ServiceFee:          "100",
		Status:              model.BtcProcessingStatusPending,
	}
	if err := db.Create(dup).Error; err == nil {
		t.Fatal("duplicate swap_transaction_hash was accepted; unique index not enforced")
	}

	// Negative control: a different swap hash inserts fine (index blocks only dups).
	other := &model.OnchainBtcProcessedTransaction{
		SwapTransactionHash: "0xswapdifferent",
		BTCAddress:          "bc1qother",
		Subtotal:            "2000",
		ServiceFee:          "100",
		Status:              model.BtcProcessingStatusPending,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("distinct swap hash should insert: %v", err)
	}
}
