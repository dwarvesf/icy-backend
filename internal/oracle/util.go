package oracle

import (
	"fmt"
	"math"
	"math/big"

	"github.com/dwarvesf/icy-backend/internal/model"
)

func getConversionRatio(circulatedIcy, btcSupply *model.Web3BigInt) (*model.Web3BigInt, error) {
	if circulatedIcy == nil || btcSupply == nil {
		return nil, fmt.Errorf("circulatedIcy or btcSupply is nil")
	}

	icyFloat := circulatedIcy.ToFloat()
	btcFloat := btcSupply.ToFloat()

	if btcFloat == 0 {
		return &model.Web3BigInt{
			Value:   "0",
			Decimal: 6,
		}, nil
	}

	ratio := icyFloat / btcFloat

	ratioFloat := new(big.Float).SetFloat64(ratio)

	multiplier := new(big.Float).SetFloat64(math.Pow(10, 6))
	ratioFloat.Mul(ratioFloat, multiplier)

	ratioInt := new(big.Int)
	ratioFloat.Int(ratioInt)
	return &model.Web3BigInt{
		Value:   ratioInt.String(),
		Decimal: 6,
	}, nil
}
