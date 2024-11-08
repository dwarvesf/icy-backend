package model

import (
	"math"
	"math/big"
)

type Web3BigInt struct {
	Value   string `json:"value"`
	Decimal int    `json:"decimal"`
}

func (w *Web3BigInt) Int64() (int64, bool) {
	amt, ok := new(big.Int).SetString(w.Value, 10)
	if !ok {
		return 0, false
	}

	return amt.Int64(), true
}

func (w *Web3BigInt) ToFloat() float64 {
	num := new(big.Int)
	num.SetString(w.Value, 10)

	floatNum := new(big.Float).SetInt(num)

	divisor := new(big.Float).SetFloat64(math.Pow(10, float64(w.Decimal)))

	floatNum.Quo(floatNum, divisor)

	result, _ := floatNum.Float64()
	return result
}

func (w *Web3BigInt) Add(number *Web3BigInt) *Web3BigInt {
	num1 := new(big.Int)
	num1.SetString(w.Value, 10)

	num2 := new(big.Int)
	num2.SetString(number.Value, 10)

	result := new(big.Int)
	result.Add(num1, num2)

	return &Web3BigInt{
		Value:   result.String(),
		Decimal: w.Decimal,
	}
}

func (w *Web3BigInt) Sub(number *Web3BigInt) *Web3BigInt {
	num1 := new(big.Int)
	num1.SetString(w.Value, 10)

	num2 := new(big.Int)
	num2.SetString(number.Value, 10)

	result := new(big.Int)
	result.Sub(num1, num2)

	return &Web3BigInt{
		Value:   result.String(),
		Decimal: w.Decimal,
	}
}
