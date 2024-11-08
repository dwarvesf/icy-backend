package btcrpc

import "github.com/dwarvesf/icy-backend/internal/model"

type IBtcRpc interface {
	Send(receiverAddress string, amount *model.Web3BigInt) error
	BalanceOf(address string) (*model.Web3BigInt, error)
}
