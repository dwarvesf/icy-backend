package baserpc_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
)

var (
	icyToken    = common.HexToAddress("0x1111111111111111111111111111111111111111")
	otherToken  = common.HexToAddress("0x2222222222222222222222222222222222222222")
	treasury    = common.HexToAddress("0x3333333333333333333333333333333333333333")
	user        = common.HexToAddress("0x4444444444444444444444444444444444444444")
	someoneElse = common.HexToAddress("0x5555555555555555555555555555555555555555")

	transferSig = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
)

// transferLog builds a well-formed ERC20 Transfer log:
// Transfer(address indexed from, address indexed to, uint256 value).
func transferLog(token, from, to common.Address, value *big.Int) *types.Log {
	return &types.Log{
		Address: token,
		Topics: []common.Hash{
			transferSig,
			common.BytesToHash(from.Bytes()), // indexed from (left-padded)
			common.BytesToHash(to.Bytes()),   // indexed to (left-padded)
		},
		Data: common.LeftPadBytes(value.Bytes(), 32), // non-indexed value
	}
}

// approvalLog is a non-Transfer log from the ICY token; ParseTransfer must skip
// it (topic[0] mismatch), so it never contributes to the deposit sum.
func approvalLog(token common.Address) *types.Log {
	approvalSig := crypto.Keccak256Hash([]byte("Approval(address,address,uint256)"))
	return &types.Log{
		Address: token,
		Topics: []common.Hash{
			approvalSig,
			common.BytesToHash(user.Bytes()),
			common.BytesToHash(treasury.Bytes()),
		},
		Data: common.LeftPadBytes(big.NewInt(999).Bytes(), 32),
	}
}

func mustSum(t *testing.T, logs []*types.Log) *big.Int {
	t.Helper()
	got, err := baserpc.SumICYTransfersTo(logs, icyToken, treasury)
	if err != nil {
		t.Fatalf("SumICYTransfersTo: %v", err)
	}
	return got
}

// The exact-amount deposit to the treasury is summed correctly (accept path).
func TestSumICYTransfersTo_ExactDepositCounted(t *testing.T) {
	logs := []*types.Log{transferLog(icyToken, user, treasury, big.NewInt(1000))}
	if got := mustSum(t, logs); got.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("sum = %s, want 1000", got)
	}
}

// Multiple ICY transfers to the treasury in one tx are summed.
func TestSumICYTransfersTo_MultipleDepositsSummed(t *testing.T) {
	logs := []*types.Log{
		transferLog(icyToken, user, treasury, big.NewInt(600)),
		transferLog(icyToken, user, treasury, big.NewInt(400)),
	}
	if got := mustSum(t, logs); got.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("sum = %s, want 1000", got)
	}
}

// A short deposit sums to less than a caller's required amount (reject path:
// the caller compares deposited < required). Negative control for the exact case.
func TestSumICYTransfersTo_ShortDepositUndercounts(t *testing.T) {
	logs := []*types.Log{transferLog(icyToken, user, treasury, big.NewInt(500))}
	got := mustSum(t, logs)
	if got.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("sum = %s, want 500", got)
	}
	if got.Cmp(big.NewInt(1000)) >= 0 {
		t.Fatalf("short deposit %s must be < required 1000", got)
	}
}

// No ICY transfer to the treasury at all -> sum is zero (absent-deposit reject).
func TestSumICYTransfersTo_AbsentDepositIsZero(t *testing.T) {
	if got := mustSum(t, nil); got.Sign() != 0 {
		t.Fatalf("sum over no logs = %s, want 0", got)
	}
}

// A transfer to a DIFFERENT recipient is not credited to the treasury.
func TestSumICYTransfersTo_WrongRecipientIgnored(t *testing.T) {
	logs := []*types.Log{transferLog(icyToken, user, someoneElse, big.NewInt(1000))}
	if got := mustSum(t, logs); got.Sign() != 0 {
		t.Fatalf("transfer to non-treasury credited: sum = %s, want 0", got)
	}
}

// A DIFFERENT token's Transfer to the treasury (log.Address != icyToken) must be
// ignored: a swap cannot be satisfied by depositing some unrelated token.
func TestSumICYTransfersTo_WrongTokenIgnored(t *testing.T) {
	logs := []*types.Log{transferLog(otherToken, user, treasury, big.NewInt(1000))}
	if got := mustSum(t, logs); got.Sign() != 0 {
		t.Fatalf("wrong-token transfer credited: sum = %s, want 0", got)
	}
}

// A non-Transfer log from the ICY token (Approval) is skipped, so only the real
// Transfer contributes.
func TestSumICYTransfersTo_NonTransferLogSkipped(t *testing.T) {
	logs := []*types.Log{
		approvalLog(icyToken),
		transferLog(icyToken, user, treasury, big.NewInt(700)),
	}
	if got := mustSum(t, logs); got.Cmp(big.NewInt(700)) != 0 {
		t.Fatalf("sum = %s, want 700 (approval ignored)", got)
	}
}
