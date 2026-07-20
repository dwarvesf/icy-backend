package onchainbtcprocessedtransaction

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct {
}

func New() IStore {
	return &store{}
}

func (s *store) Create(tx *gorm.DB, btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error) {
	btcProcessedTx.CreatedAt = time.Now()
	btcProcessedTx.UpdatedAt = time.Now()
	return btcProcessedTx, tx.Create(btcProcessedTx).Error
}

func (s *store) GetByIcyTransactionHash(tx *gorm.DB, icyTxHash string) (*model.OnchainBtcProcessedTransaction, error) {
	var btcProcessedTx model.OnchainBtcProcessedTransaction
	result := tx.Where("icy_transaction_hash = ?", icyTxHash).First(&btcProcessedTx)
	if result.Error != nil {
		return nil, result.Error
	}
	return &btcProcessedTx, nil
}

// GetBySwapTransactionHash looks up the payout row for a given on-chain ICY swap
// transaction. This is the dedup key that is ACTUALLY populated on every payout
// (swap_transaction_hash = the Swap event's tx hash) and is backed by the
// partial UNIQUE index from migration 0013. The indexer calls this before
// creating a payout so a re-indexed / reorg-re-emitted swap event never mints a
// second BTC payout. Returns gorm.ErrRecordNotFound when no payout exists yet.
func (s *store) GetBySwapTransactionHash(tx *gorm.DB, swapTxHash string) (*model.OnchainBtcProcessedTransaction, error) {
	var btcProcessedTx model.OnchainBtcProcessedTransaction
	result := tx.Where("swap_transaction_hash = ?", swapTxHash).First(&btcProcessedTx)
	if result.Error != nil {
		return nil, result.Error
	}
	return &btcProcessedTx, nil
}

// ClaimPendingTransaction atomically transitions a single pending row to
// "processing" via a conditional UPDATE (WHERE id = ? AND status = 'pending').
// It returns true ONLY when this call was the one that flipped the row
// (RowsAffected == 1). Because the WHERE clause and the write are one atomic
// statement, at most one caller can ever win a given row, even when two cron
// cycles overlap. A row already processing / completed / failed yields false
// with no error, so the caller must skip it (do NOT broadcast). This is the
// exactly-once gate for BTC settlement: a payout is broadcast only by the
// claim winner, and a crash after broadcast leaves the row in "processing"
// (never pending again), so it is never re-sent.
func (s *store) ClaimPendingTransaction(tx *gorm.DB, id int) (bool, error) {
	result := tx.Model(&model.OnchainBtcProcessedTransaction{}).
		Where("id = ? AND status = ?", id, model.BtcProcessingStatusPending).
		Updates(map[string]interface{}{
			"status":     model.BtcProcessingStatusProcessing,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ReleaseForRetry returns a claimed row to pending and counts the attempt in
// the SAME statement, so a crash between the two cannot lose the count and let
// a row retry unboundedly. Returns the new attempt total.
func (s *store) ReleaseForRetry(tx *gorm.DB, id int) (int, error) {
	if err := tx.Model(&model.OnchainBtcProcessedTransaction{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     model.BtcProcessingStatusPending,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": time.Now(),
		}).Error; err != nil {
		return 0, err
	}
	var row model.OnchainBtcProcessedTransaction
	if err := tx.Select("attempts").First(&row, "id = ?", id).Error; err != nil {
		return 0, err
	}
	return row.Attempts, nil
}

func (s *store) UpdateStatus(tx *gorm.DB, id int, status model.BtcProcessingStatus) error {
	return tx.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error
}

func (s *store) UpdateToCompleted(tx *gorm.DB, id int, btcTxHash string, networkFee int64) error {
	return tx.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":               model.BtcProcessingStatusCompleted,
		"btc_transaction_hash": btcTxHash,
		"network_fee":          fmt.Sprintf("%d", networkFee),
		"updated_at":           time.Now(),
		"processed_at":         time.Now(),
	}).Error
}

// UpdateToBroadcasted records a successful broadcast without completing the row.
// It stores the on-chain tx hash + network fee and stamps processed_at (the
// broadcast time, used by the confirmation sweep's stuck-timeout), but leaves the
// row in the non-terminal "broadcasted" state so it is NOT treated as settled
// until it reaches MinBtcConfirmations. Deliberately does NOT touch pending, so a
// broadcasted row is never re-claimed / re-broadcast (no double-send).
func (s *store) UpdateToBroadcasted(tx *gorm.DB, id int, btcTxHash string, networkFee int64) error {
	return tx.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":               model.BtcProcessingStatusBroadcasted,
		"btc_transaction_hash": btcTxHash,
		"network_fee":          fmt.Sprintf("%d", networkFee),
		"updated_at":           time.Now(),
		"processed_at":         time.Now(),
	}).Error
}

func (s *store) GetBroadcastedTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error) {
	var broadcastedTxs []model.OnchainBtcProcessedTransaction
	err := tx.Where("status = ?", model.BtcProcessingStatusBroadcasted).Find(&broadcastedTxs).Error
	return broadcastedTxs, err
}

func (s *store) GetPendingTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error) {
	var pendingTxs []model.OnchainBtcProcessedTransaction
	err := tx.Where("status = ?", model.BtcProcessingStatusPending).Find(&pendingTxs).Error
	return pendingTxs, err
}

// sentStates are the payout states that represent BTC that HAS (or MAY have)
// left the treasury and therefore count toward the rolling daily cap:
//   - broadcasted / completed: the tx was definitely put on the wire.
//   - needs_reconcile: AMBIGUOUS (a POST was attempted, the tx may be live) or a
//     stuck broadcast. Counted so the daily total is a true CEILING on outflow;
//     over-counting a maybe-sent row is the safe direction for a drain-prevention
//     control. "pending" / "processing" / "failed" are excluded (not sent).
//
// "refused" is DELIBERATELY excluded. It marks a payout a policy control
// definitively never broadcast. It used to share needs_reconcile, so a
// cap refusal counted its own amount as outflow, pushing the next payout over
// the cap, which refused and counted again: the ceiling ratcheted itself shut
// for 24 hours with nothing actually sent. Never add it to this list.
var sentStates = []string{
	string(model.BtcProcessingStatusBroadcasted),
	string(model.BtcProcessingStatusCompleted),
	string(model.BtcProcessingStatusNeedsReconcile),
}

// SumSentInWindow returns the total sendable amount, in satoshi, of every payout
// in a sent state (see sentStates) whose send timestamp falls at or after `since`.
// The per-row sendable amount is subtotal - service_fee (the exact figure that
// left, or would have left, the treasury). The send timestamp is processed_at
// (the broadcast/completion time) when set, else updated_at, so an ambiguous
// needs_reconcile row that never recorded a broadcast time still counts via its
// last-update time (conservative). Amounts are stored as strings; a row whose
// amount cannot be parsed returns an error rather than being skipped, so the
// caller FAILS CLOSED (refuses the payout) instead of under-counting the daily
// total. This is the rolling-24h input for the SG-06 daily payout cap.
func (s *store) SumSentInWindow(tx *gorm.DB, since time.Time) (int64, error) {
	var rows []model.OnchainBtcProcessedTransaction
	if err := tx.Where("status IN ?", sentStates).Find(&rows).Error; err != nil {
		return 0, err
	}

	var total int64
	for _, r := range rows {
		sentAt := r.UpdatedAt
		if r.ProcessedAt != nil {
			sentAt = *r.ProcessedAt
		}
		if sentAt.Before(since) {
			continue
		}

		subtotal, err := strconv.ParseInt(r.Subtotal, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("SumSentInWindow: parse subtotal %q (id %d): %w", r.Subtotal, r.ID, err)
		}
		var svcFee int64
		if r.ServiceFee != "" {
			svcFee, err = strconv.ParseInt(r.ServiceFee, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("SumSentInWindow: parse service_fee %q (id %d): %w", r.ServiceFee, r.ID, err)
			}
		}

		amount := subtotal - svcFee
		if amount < 0 {
			// A negative row never sent BTC; do not let it reduce the running total.
			amount = 0
		}
		total += amount
	}

	return total, nil
}

func (s *store) Find(db *gorm.DB, filter ListFilter) ([]*model.OnchainBtcProcessedTransaction, int64, error) {
	var transactions []*model.OnchainBtcProcessedTransaction
	var total int64

	// Start with base query
	query := db.Model(&model.OnchainBtcProcessedTransaction{})

	// Apply filters
	if filter.BTCAddress != "" {
		query = query.Where("LOWER(btc_address) = ?", strings.ToLower(filter.BTCAddress))
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EVMAddress != "" {
		query = query.Joins("LEFT JOIN onchain_icy_swap_transactions ON onchain_icy_swap_transactions.transaction_hash = onchain_btc_processed_transactions.swap_transaction_hash").
			Where("LOWER(onchain_icy_swap_transactions.from_address) = ?", strings.ToLower(filter.EVMAddress))
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Prepare final query with preloading
	finalQuery := db.Model(&model.OnchainBtcProcessedTransaction{}).
		Preload("OnchainIcySwapTransaction")

	// Reapply all filters to final query
	if filter.BTCAddress != "" {
		finalQuery = finalQuery.Where("LOWER(btc_address) = ?", strings.ToLower(filter.BTCAddress))
	}
	if filter.Status != "" {
		finalQuery = finalQuery.Where("status = ?", filter.Status)
	}
	if filter.EVMAddress != "" {
		finalQuery = finalQuery.Joins("LEFT JOIN onchain_icy_swap_transactions ON onchain_icy_swap_transactions.transaction_hash = onchain_btc_processed_transactions.swap_transaction_hash").
			Where("LOWER(onchain_icy_swap_transactions.from_address) = ?", strings.ToLower(filter.EVMAddress))
	}

	// Apply pagination and ordering
	finalQuery = finalQuery.
		Offset(filter.Offset).
		Limit(filter.Limit).
		Order("updated_at DESC")

	// Fetch transactions
	if err := finalQuery.Find(&transactions).Error; err != nil {
		return nil, 0, err
	}

	for i := range transactions {
		subtotal, err := strconv.ParseInt(transactions[i].Subtotal, 10, 64)
		if err != nil {
			continue
		}
		svcFee, err := strconv.ParseInt(transactions[i].ServiceFee, 10, 64)
		if err != nil {
			continue
		}
		totalAmount := subtotal - svcFee
		transactions[i].Total = strconv.FormatInt(totalAmount, 10)
	}

	return transactions, total, nil
}
