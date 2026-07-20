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

// ErrUnsupportedAddressType is returned for a mainnet address whose type
// cannot be paid to usefully (a bare public key, producing a P2PK output).
var ErrUnsupportedAddressType = errors.New("bitcoin address type is not supported")

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

	// DecodeAddress also accepts a raw serialized public key (66 or 130 hex
	// chars) and returns an AddressPubKey, which IsForNet confirms as mainnet.
	// Paying that produces a bare P2PK output that most wallets and every
	// exchange cannot display or spend, so the funds are effectively stranded.
	// Allow only the address types a normal recipient can actually use.
	switch decoded.(type) {
	case *btcutil.AddressPubKeyHash, // 1...
		*btcutil.AddressScriptHash,        // 3...
		*btcutil.AddressWitnessPubKeyHash, // bc1q...
		*btcutil.AddressWitnessScriptHash, // bc1q... (script)
		*btcutil.AddressTaproot:           // bc1p...
		return nil
	default:
		return ErrUnsupportedAddressType
	}
}

// NormalizeAddress returns the form that must be used everywhere downstream.
//
// Validation trims, so a padded address passes and then a DIFFERENT string is
// hashed into the swap signature, missed by getDustLimit's prefix table, and
// finally rejected by Send, after the ICY leg has already burned. Callers
// normalize once at the edge and use only the result.
func NormalizeAddress(addr string) string {
	return strings.TrimSpace(addr)
}
