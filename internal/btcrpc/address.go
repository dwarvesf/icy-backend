package btcrpc

import (
	"errors"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// ErrNotMainnetAddress is returned for a syntactically valid Bitcoin address
// that belongs to a network we do not pay from (testnet, signet, regtest).
var ErrNotMainnetAddress = errors.New("bitcoin address is not on mainnet")

// ValidateMainnetAddress parses addr and confirms it is a Bitcoin MAINNET
// address of a type we can pay to.
//
// Payouts settle on mainnet, so signing for a testnet or regtest address is a
// guaranteed loss: the ICY leg burns on Base and the BTC leg targets an
// encoding that cannot be paid. Prefix checks are not enough, getDustLimit's
// prefix table falls through to a 546-sat default for tb1, bcrt1 and outright
// garbage alike, so an unvalidated address reaches the signer intact.
func ValidateMainnetAddress(addr string) error {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return errors.New("bitcoin address is empty")
	}

	// DecodeAddress fails when the address does not belong to the given
	// network, which is what rejects tb1/bcrt1/m/n here rather than a prefix
	// table. It also rejects bad checksums and unknown encodings.
	decoded, err := btcutil.DecodeAddress(trimmed, &chaincfg.MainNetParams)
	if err != nil {
		return ErrNotMainnetAddress
	}
	if !decoded.IsForNet(&chaincfg.MainNetParams) {
		return ErrNotMainnetAddress
	}
	return nil
}
