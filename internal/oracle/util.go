package oracle

import (
	"fmt"
	"math/big"

	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
)

// getConversionRatio calculates the ratio of circulated ICY tokens to BTC supply
// The ratio is scaled by 10^8 to preserve 8 decimal places of precision
func getConversionRatio(circulatedIcy, btcSupply *model.Web3BigInt) (*model.Web3BigInt, error) {
	if circulatedIcy == nil || btcSupply == nil {
		return nil, fmt.Errorf("circulatedIcy or btcSupply is nil")
	}

	// Convert inputs to big.Float for high-precision calculation
	icyFloat := new(big.Float).SetFloat64(circulatedIcy.ToFloat())
	btcFloat := new(big.Float).SetFloat64(btcSupply.ToFloat())

	// Handle zero BTC supply case
	if btcFloat.Cmp(new(big.Float).SetFloat64(0)) == 0 {
		return &model.Web3BigInt{
			Value:   "0",
			Decimal: consts.BTC_DECIMALS, // Using 8 decimals to match the 10^8 scaling factor
		}, nil
	}

	// Calculate ratio and scale by 10^8
	ratio := new(big.Float).Quo(icyFloat, btcFloat)
	multiplier := new(big.Float).SetFloat64(1e8)
	scaledRatio := new(big.Float).Mul(ratio, multiplier)

	// Convert to integer representation
	ratioInt := new(big.Int)
	scaledRatio.Int(ratioInt)

	return &model.Web3BigInt{
		Value:   ratioInt.String(),
		Decimal: consts.BTC_DECIMALS, // Using 8 decimals to match the 10^8 scaling factor
	}, nil
}
