package oracle

import (
	"math"
	"math/big"

	"github.com/dwarvesf/icy-backend/internal/model"
)

func getConversionRatio(circulatedIcy, btcSupply *model.Web3BigInt) (*model.Web3BigInt, error) {
	icyFloat := circulatedIcy.ToFloat()
	btcFloat := btcSupply.ToFloat()

	if btcFloat == 0 {
		return nil, ErrBtcSupplyZero
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
