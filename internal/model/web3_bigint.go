package model

import (
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

// TODO: add other utility functions for Web3BigInt
