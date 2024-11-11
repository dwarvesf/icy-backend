package baserpc

import "github.com/dwarvesf/icy-backend/internal/model"

type IBaseRPC interface {
	ICYBalanceOf(address string) (*model.Web3BigInt, error)
	ICYTotalSupply() (*model.Web3BigInt, error)
}
