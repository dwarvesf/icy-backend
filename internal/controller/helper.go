package controller

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/dwarvesf/icy-backend/internal/model"
)

// Helper functions
func (c *Controller) isPriceChangedSignificantly(current, cached *model.Web3BigInt) bool {
	if cached == nil {
		return false
	}

	currentFloat := current.ToFloat()
	cachedFloat := cached.ToFloat()

	// Calculate percentage change
	change := ((currentFloat - cachedFloat) / cachedFloat) * 100
	return change >= 5 || change <= -5 // 5% threshold
}

func (c *Controller) hasSufficientBalance(balance, required *model.Web3BigInt) bool {
	balanceFloat := balance.ToFloat()
	requiredFloat := required.ToFloat()
	return balanceFloat >= requiredFloat
}

// bech32Charset is the character set for Bech32 encoding.
const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// base58Alphabet is the alphabet used for Base58 encoding.
var base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// validateBTCAddress checks whether the given Bitcoin address is valid.
// It returns nil if the address is valid, otherwise an error describing the issue.
// It supports legacy addresses (Base58Check encoded, starting with '1' or '3')
// and bech32 addresses (starting with "bc1").
func (c *Controller) validateBTCAddress(address string) error {
	if len(address) == 0 {
		return errors.New("address is empty")
	}

	if c.config.Environment != "prod" && c.config.Environment != "production" {
		return nil
	}

	// Legacy addresses: start with '1' (P2PKH) or '3' (P2SH)
	if address[0] == '1' || address[0] == '3' {
		return c.validateLegacyBTCAddress(address)
	}

	// Bech32 addresses: start with "bc1" (mainnet)
	if strings.HasPrefix(strings.ToLower(address), "bc1") {
		return c.validateBech32BTCAddress(address)
	}

	return errors.New("address does not start with a recognized prefix")
}

// validateLegacyBTCAddress validates a legacy Bitcoin address (Base58Check).
func (c *Controller) validateLegacyBTCAddress(address string) error {
	decoded, err := c.base58Decode(address)
	if err != nil {
		return fmt.Errorf("base58 decode error: %v", err)
	}

	// The decoded address must have at least 1 version byte + 1 payload byte + 4 checksum bytes.
	if len(decoded) < 5 {
		return errors.New("decoded address is too short")
	}

	// Split into payload and checksum.
	payload := decoded[:len(decoded)-4]
	checksumBytes := decoded[len(decoded)-4:]
	computedChecksum := c.doubleSha256Checksum(payload)
	if !bytes.Equal(checksumBytes, computedChecksum) {
		return errors.New("checksum mismatch")
	}

	// Check the version byte.
	version := payload[0]
	if address[0] == '1' && version != 0x00 {
		return fmt.Errorf("invalid version for P2PKH address: expected 0x00, got 0x%02x", version)
	}
	if address[0] == '3' && version != 0x05 {
		return fmt.Errorf("invalid version for P2SH address: expected 0x05, got 0x%02x", version)
	}

	return nil
}

// base58Decode decodes a Base58-encoded string into a byte slice.
func (c *Controller) base58Decode(input string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(58)
	for _, c := range input {
		charIndex := strings.IndexRune(base58Alphabet, c)
		if charIndex == -1 {
			return nil, fmt.Errorf("invalid character '%c' in base58 string", c)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(charIndex)))
	}
	// Convert the big.Int to bytes.
	decoded := result.Bytes()

	// Add leading zero bytes for each '1' at the beginning of the input.
	nZeros := 0
	for _, c := range input {
		if c == '1' {
			nZeros++
		} else {
			break
		}
	}
	decoded = append(make([]byte, nZeros), decoded...)
	return decoded, nil
}

// doubleSha256Checksum returns the first 4 bytes of the double SHA256 hash of data.
func (c *Controller) doubleSha256Checksum(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:4]
}

// validateBech32BTCAddress validates a Bech32 Bitcoin address.
func (c *Controller) validateBech32BTCAddress(address string) error {
	// Bech32 addresses must be all lower-case or all upper-case.
	if address != strings.ToLower(address) && address != strings.ToUpper(address) {
		return errors.New("address must be all lower-case or all upper-case")
	}
	address = strings.ToLower(address)

	// The separator '1' must exist and be in a valid position.
	sepPos := strings.LastIndex(address, "1")
	if sepPos < 1 || sepPos+7 > len(address) {
		return errors.New("separator '1' is in an invalid position")
	}
	hrp := address[:sepPos]
	// For Bitcoin mainnet Bech32 addresses, the HRP should be "bc".
	if hrp != "bc" {
		return fmt.Errorf("invalid human-readable part (expected 'bc', got %q)", hrp)
	}
	dataPart := address[sepPos+1:]
	data := make([]int, len(dataPart))
	for i, c := range dataPart {
		index := strings.IndexRune(bech32Charset, c)
		if index == -1 {
			return fmt.Errorf("invalid character in data part: %c", c)
		}
		data[i] = index
	}

	// Compute the checksum using the Bech32 algorithm.
	hrpExpanded := c.hrpExpand(hrp)
	combined := append(hrpExpanded, data...)
	if c.polymod(combined) != 1 {
		return errors.New("invalid checksum")
	}

	return nil
}

// hrpExpand expands the human-readable part (HRP) into a slice of integers.
func (c *Controller) hrpExpand(hrp string) []int {
	result := make([]int, 0, len(hrp)*2+1)
	for _, c := range hrp {
		result = append(result, int(c)>>5)
	}
	result = append(result, 0)
	for _, c := range hrp {
		result = append(result, int(c)&31)
	}
	return result
}

// polymod computes the Bech32 checksum.
func (c *Controller) polymod(values []int) int {
	chk := 1
	generator := []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	for _, v := range values {
		top := chk >> 25
		chk = ((chk & 0x1ffffff) << 5) ^ v
		for i := 0; i < 5; i++ {
			if ((top >> i) & 1) == 1 {
				chk ^= generator[i]
			}
		}
	}
	return chk
}
