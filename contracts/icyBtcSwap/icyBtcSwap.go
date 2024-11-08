// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package icyBtcSwap

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// IcyBtcSwapMetaData contains all meta data concerning the IcyBtcSwap contract.
var IcyBtcSwapMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_icy\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"}],\"name\":\"RevertIcy\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"signerAddress\",\"type\":\"address\"}],\"name\":\"SetSigner\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"}],\"name\":\"Swap\",\"type\":\"event\"},{\"stateMutability\":\"nonpayable\",\"type\":\"fallback\"},{\"inputs\":[],\"name\":\"REVERT_ICY_HASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SWAP_HASH\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"eip712Domain\",\"outputs\":[{\"internalType\":\"bytes1\",\"name\":\"fields\",\"type\":\"bytes1\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"version\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"verifyingContract\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"},{\"internalType\":\"uint256[]\",\"name\":\"extensions\",\"type\":\"uint256[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"}],\"name\":\"getRevertIcyHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"hash\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"_digest\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"_signature\",\"type\":\"bytes\"}],\"name\":\"getSigner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"}],\"name\":\"getSwapHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"hash\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"icy\",\"outputs\":[{\"internalType\":\"contractERC20\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"_signature\",\"type\":\"bytes\"}],\"name\":\"revertIcy\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"revertedIcyHashes\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_signerAddress\",\"type\":\"address\"}],\"name\":\"setSigner\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"signerAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"icyAmount\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"btcAddress\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"btcAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"_signature\",\"type\":\"bytes\"}],\"name\":\"swap\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"swappedHashes\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]",
}

// IcyBtcSwapABI is the input ABI used to generate the binding from.
// Deprecated: Use IcyBtcSwapMetaData.ABI instead.
var IcyBtcSwapABI = IcyBtcSwapMetaData.ABI

// IcyBtcSwap is an auto generated Go binding around an Ethereum contract.
type IcyBtcSwap struct {
	IcyBtcSwapCaller     // Read-only binding to the contract
	IcyBtcSwapTransactor // Write-only binding to the contract
	IcyBtcSwapFilterer   // Log filterer for contract events
}

// IcyBtcSwapCaller is an auto generated read-only Go binding around an Ethereum contract.
type IcyBtcSwapCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IcyBtcSwapTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IcyBtcSwapTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IcyBtcSwapFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IcyBtcSwapFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IcyBtcSwapSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IcyBtcSwapSession struct {
	Contract     *IcyBtcSwap       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IcyBtcSwapCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IcyBtcSwapCallerSession struct {
	Contract *IcyBtcSwapCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// IcyBtcSwapTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IcyBtcSwapTransactorSession struct {
	Contract     *IcyBtcSwapTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IcyBtcSwapRaw is an auto generated low-level Go binding around an Ethereum contract.
type IcyBtcSwapRaw struct {
	Contract *IcyBtcSwap // Generic contract binding to access the raw methods on
}

// IcyBtcSwapCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IcyBtcSwapCallerRaw struct {
	Contract *IcyBtcSwapCaller // Generic read-only contract binding to access the raw methods on
}

// IcyBtcSwapTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IcyBtcSwapTransactorRaw struct {
	Contract *IcyBtcSwapTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIcyBtcSwap creates a new instance of IcyBtcSwap, bound to a specific deployed contract.
func NewIcyBtcSwap(address common.Address, backend bind.ContractBackend) (*IcyBtcSwap, error) {
	contract, err := bindIcyBtcSwap(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwap{IcyBtcSwapCaller: IcyBtcSwapCaller{contract: contract}, IcyBtcSwapTransactor: IcyBtcSwapTransactor{contract: contract}, IcyBtcSwapFilterer: IcyBtcSwapFilterer{contract: contract}}, nil
}

// NewIcyBtcSwapCaller creates a new read-only instance of IcyBtcSwap, bound to a specific deployed contract.
func NewIcyBtcSwapCaller(address common.Address, caller bind.ContractCaller) (*IcyBtcSwapCaller, error) {
	contract, err := bindIcyBtcSwap(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapCaller{contract: contract}, nil
}

// NewIcyBtcSwapTransactor creates a new write-only instance of IcyBtcSwap, bound to a specific deployed contract.
func NewIcyBtcSwapTransactor(address common.Address, transactor bind.ContractTransactor) (*IcyBtcSwapTransactor, error) {
	contract, err := bindIcyBtcSwap(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapTransactor{contract: contract}, nil
}

// NewIcyBtcSwapFilterer creates a new log filterer instance of IcyBtcSwap, bound to a specific deployed contract.
func NewIcyBtcSwapFilterer(address common.Address, filterer bind.ContractFilterer) (*IcyBtcSwapFilterer, error) {
	contract, err := bindIcyBtcSwap(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapFilterer{contract: contract}, nil
}

// bindIcyBtcSwap binds a generic wrapper to an already deployed contract.
func bindIcyBtcSwap(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IcyBtcSwapMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IcyBtcSwap *IcyBtcSwapRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IcyBtcSwap.Contract.IcyBtcSwapCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IcyBtcSwap *IcyBtcSwapRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.IcyBtcSwapTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IcyBtcSwap *IcyBtcSwapRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.IcyBtcSwapTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IcyBtcSwap *IcyBtcSwapCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IcyBtcSwap.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IcyBtcSwap *IcyBtcSwapTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IcyBtcSwap *IcyBtcSwapTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.contract.Transact(opts, method, params...)
}

// REVERTICYHASH is a free data retrieval call binding the contract method 0x2d6d3d01.
//
// Solidity: function REVERT_ICY_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapCaller) REVERTICYHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "REVERT_ICY_HASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// REVERTICYHASH is a free data retrieval call binding the contract method 0x2d6d3d01.
//
// Solidity: function REVERT_ICY_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapSession) REVERTICYHASH() ([32]byte, error) {
	return _IcyBtcSwap.Contract.REVERTICYHASH(&_IcyBtcSwap.CallOpts)
}

// REVERTICYHASH is a free data retrieval call binding the contract method 0x2d6d3d01.
//
// Solidity: function REVERT_ICY_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) REVERTICYHASH() ([32]byte, error) {
	return _IcyBtcSwap.Contract.REVERTICYHASH(&_IcyBtcSwap.CallOpts)
}

// SWAPHASH is a free data retrieval call binding the contract method 0x30c8b3da.
//
// Solidity: function SWAP_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapCaller) SWAPHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "SWAP_HASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// SWAPHASH is a free data retrieval call binding the contract method 0x30c8b3da.
//
// Solidity: function SWAP_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapSession) SWAPHASH() ([32]byte, error) {
	return _IcyBtcSwap.Contract.SWAPHASH(&_IcyBtcSwap.CallOpts)
}

// SWAPHASH is a free data retrieval call binding the contract method 0x30c8b3da.
//
// Solidity: function SWAP_HASH() view returns(bytes32)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) SWAPHASH() ([32]byte, error) {
	return _IcyBtcSwap.Contract.SWAPHASH(&_IcyBtcSwap.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_IcyBtcSwap *IcyBtcSwapCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "eip712Domain")

	outstruct := new(struct {
		Fields            [1]byte
		Name              string
		Version           string
		ChainId           *big.Int
		VerifyingContract common.Address
		Salt              [32]byte
		Extensions        []*big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)

	return *outstruct, err

}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_IcyBtcSwap *IcyBtcSwapSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _IcyBtcSwap.Contract.Eip712Domain(&_IcyBtcSwap.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _IcyBtcSwap.Contract.Eip712Domain(&_IcyBtcSwap.CallOpts)
}

// GetRevertIcyHash is a free data retrieval call binding the contract method 0x32ce558d.
//
// Solidity: function getRevertIcyHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapCaller) GetRevertIcyHash(opts *bind.CallOpts, icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "getRevertIcyHash", icyAmount, btcAddress, btcAmount, nonce, deadline)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetRevertIcyHash is a free data retrieval call binding the contract method 0x32ce558d.
//
// Solidity: function getRevertIcyHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapSession) GetRevertIcyHash(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	return _IcyBtcSwap.Contract.GetRevertIcyHash(&_IcyBtcSwap.CallOpts, icyAmount, btcAddress, btcAmount, nonce, deadline)
}

// GetRevertIcyHash is a free data retrieval call binding the contract method 0x32ce558d.
//
// Solidity: function getRevertIcyHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) GetRevertIcyHash(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	return _IcyBtcSwap.Contract.GetRevertIcyHash(&_IcyBtcSwap.CallOpts, icyAmount, btcAddress, btcAmount, nonce, deadline)
}

// GetSigner is a free data retrieval call binding the contract method 0xf7b2ec0d.
//
// Solidity: function getSigner(bytes32 _digest, bytes _signature) view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCaller) GetSigner(opts *bind.CallOpts, _digest [32]byte, _signature []byte) (common.Address, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "getSigner", _digest, _signature)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetSigner is a free data retrieval call binding the contract method 0xf7b2ec0d.
//
// Solidity: function getSigner(bytes32 _digest, bytes _signature) view returns(address)
func (_IcyBtcSwap *IcyBtcSwapSession) GetSigner(_digest [32]byte, _signature []byte) (common.Address, error) {
	return _IcyBtcSwap.Contract.GetSigner(&_IcyBtcSwap.CallOpts, _digest, _signature)
}

// GetSigner is a free data retrieval call binding the contract method 0xf7b2ec0d.
//
// Solidity: function getSigner(bytes32 _digest, bytes _signature) view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) GetSigner(_digest [32]byte, _signature []byte) (common.Address, error) {
	return _IcyBtcSwap.Contract.GetSigner(&_IcyBtcSwap.CallOpts, _digest, _signature)
}

// GetSwapHash is a free data retrieval call binding the contract method 0x6327a9d0.
//
// Solidity: function getSwapHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapCaller) GetSwapHash(opts *bind.CallOpts, icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "getSwapHash", icyAmount, btcAddress, btcAmount, nonce, deadline)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetSwapHash is a free data retrieval call binding the contract method 0x6327a9d0.
//
// Solidity: function getSwapHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapSession) GetSwapHash(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	return _IcyBtcSwap.Contract.GetSwapHash(&_IcyBtcSwap.CallOpts, icyAmount, btcAddress, btcAmount, nonce, deadline)
}

// GetSwapHash is a free data retrieval call binding the contract method 0x6327a9d0.
//
// Solidity: function getSwapHash(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline) view returns(bytes32 hash)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) GetSwapHash(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int) ([32]byte, error) {
	return _IcyBtcSwap.Contract.GetSwapHash(&_IcyBtcSwap.CallOpts, icyAmount, btcAddress, btcAmount, nonce, deadline)
}

// Icy is a free data retrieval call binding the contract method 0x7f245ab1.
//
// Solidity: function icy() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCaller) Icy(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "icy")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Icy is a free data retrieval call binding the contract method 0x7f245ab1.
//
// Solidity: function icy() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapSession) Icy() (common.Address, error) {
	return _IcyBtcSwap.Contract.Icy(&_IcyBtcSwap.CallOpts)
}

// Icy is a free data retrieval call binding the contract method 0x7f245ab1.
//
// Solidity: function icy() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) Icy() (common.Address, error) {
	return _IcyBtcSwap.Contract.Icy(&_IcyBtcSwap.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapSession) Owner() (common.Address, error) {
	return _IcyBtcSwap.Contract.Owner(&_IcyBtcSwap.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) Owner() (common.Address, error) {
	return _IcyBtcSwap.Contract.Owner(&_IcyBtcSwap.CallOpts)
}

// RevertedIcyHashes is a free data retrieval call binding the contract method 0x3d2b52db.
//
// Solidity: function revertedIcyHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapCaller) RevertedIcyHashes(opts *bind.CallOpts, arg0 [32]byte) (bool, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "revertedIcyHashes", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// RevertedIcyHashes is a free data retrieval call binding the contract method 0x3d2b52db.
//
// Solidity: function revertedIcyHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapSession) RevertedIcyHashes(arg0 [32]byte) (bool, error) {
	return _IcyBtcSwap.Contract.RevertedIcyHashes(&_IcyBtcSwap.CallOpts, arg0)
}

// RevertedIcyHashes is a free data retrieval call binding the contract method 0x3d2b52db.
//
// Solidity: function revertedIcyHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) RevertedIcyHashes(arg0 [32]byte) (bool, error) {
	return _IcyBtcSwap.Contract.RevertedIcyHashes(&_IcyBtcSwap.CallOpts, arg0)
}

// SignerAddress is a free data retrieval call binding the contract method 0x5b7633d0.
//
// Solidity: function signerAddress() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCaller) SignerAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "signerAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SignerAddress is a free data retrieval call binding the contract method 0x5b7633d0.
//
// Solidity: function signerAddress() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapSession) SignerAddress() (common.Address, error) {
	return _IcyBtcSwap.Contract.SignerAddress(&_IcyBtcSwap.CallOpts)
}

// SignerAddress is a free data retrieval call binding the contract method 0x5b7633d0.
//
// Solidity: function signerAddress() view returns(address)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) SignerAddress() (common.Address, error) {
	return _IcyBtcSwap.Contract.SignerAddress(&_IcyBtcSwap.CallOpts)
}

// SwappedHashes is a free data retrieval call binding the contract method 0x6072e236.
//
// Solidity: function swappedHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapCaller) SwappedHashes(opts *bind.CallOpts, arg0 [32]byte) (bool, error) {
	var out []interface{}
	err := _IcyBtcSwap.contract.Call(opts, &out, "swappedHashes", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SwappedHashes is a free data retrieval call binding the contract method 0x6072e236.
//
// Solidity: function swappedHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapSession) SwappedHashes(arg0 [32]byte) (bool, error) {
	return _IcyBtcSwap.Contract.SwappedHashes(&_IcyBtcSwap.CallOpts, arg0)
}

// SwappedHashes is a free data retrieval call binding the contract method 0x6072e236.
//
// Solidity: function swappedHashes(bytes32 ) view returns(bool)
func (_IcyBtcSwap *IcyBtcSwapCallerSession) SwappedHashes(arg0 [32]byte) (bool, error) {
	return _IcyBtcSwap.Contract.SwappedHashes(&_IcyBtcSwap.CallOpts, arg0)
}

// RevertIcy is a paid mutator transaction binding the contract method 0x666f5a65.
//
// Solidity: function revertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) RevertIcy(opts *bind.TransactOpts, icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.Transact(opts, "revertIcy", icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// RevertIcy is a paid mutator transaction binding the contract method 0x666f5a65.
//
// Solidity: function revertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapSession) RevertIcy(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.RevertIcy(&_IcyBtcSwap.TransactOpts, icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// RevertIcy is a paid mutator transaction binding the contract method 0x666f5a65.
//
// Solidity: function revertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) RevertIcy(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.RevertIcy(&_IcyBtcSwap.TransactOpts, icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// SetSigner is a paid mutator transaction binding the contract method 0x6c19e783.
//
// Solidity: function setSigner(address _signerAddress) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) SetSigner(opts *bind.TransactOpts, _signerAddress common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.Transact(opts, "setSigner", _signerAddress)
}

// SetSigner is a paid mutator transaction binding the contract method 0x6c19e783.
//
// Solidity: function setSigner(address _signerAddress) returns()
func (_IcyBtcSwap *IcyBtcSwapSession) SetSigner(_signerAddress common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.SetSigner(&_IcyBtcSwap.TransactOpts, _signerAddress)
}

// SetSigner is a paid mutator transaction binding the contract method 0x6c19e783.
//
// Solidity: function setSigner(address _signerAddress) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) SetSigner(_signerAddress common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.SetSigner(&_IcyBtcSwap.TransactOpts, _signerAddress)
}

// Swap is a paid mutator transaction binding the contract method 0xade44138.
//
// Solidity: function swap(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) Swap(opts *bind.TransactOpts, icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.Transact(opts, "swap", icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// Swap is a paid mutator transaction binding the contract method 0xade44138.
//
// Solidity: function swap(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapSession) Swap(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Swap(&_IcyBtcSwap.TransactOpts, icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// Swap is a paid mutator transaction binding the contract method 0xade44138.
//
// Solidity: function swap(uint256 icyAmount, string btcAddress, uint256 btcAmount, uint256 nonce, uint256 deadline, bytes _signature) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) Swap(icyAmount *big.Int, btcAddress string, btcAmount *big.Int, nonce *big.Int, deadline *big.Int, _signature []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Swap(&_IcyBtcSwap.TransactOpts, icyAmount, btcAddress, btcAmount, nonce, deadline, _signature)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IcyBtcSwap *IcyBtcSwapSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.TransferOwnership(&_IcyBtcSwap.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.TransferOwnership(&_IcyBtcSwap.TransactOpts, newOwner)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) Fallback(opts *bind.TransactOpts, calldata []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.RawTransact(opts, calldata)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() returns()
func (_IcyBtcSwap *IcyBtcSwapSession) Fallback(calldata []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Fallback(&_IcyBtcSwap.TransactOpts, calldata)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) Fallback(calldata []byte) (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Fallback(&_IcyBtcSwap.TransactOpts, calldata)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_IcyBtcSwap *IcyBtcSwapTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IcyBtcSwap.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_IcyBtcSwap *IcyBtcSwapSession) Receive() (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Receive(&_IcyBtcSwap.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_IcyBtcSwap *IcyBtcSwapTransactorSession) Receive() (*types.Transaction, error) {
	return _IcyBtcSwap.Contract.Receive(&_IcyBtcSwap.TransactOpts)
}

// IcyBtcSwapOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the IcyBtcSwap contract.
type IcyBtcSwapOwnershipTransferredIterator struct {
	Event *IcyBtcSwapOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IcyBtcSwapOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IcyBtcSwapOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IcyBtcSwapOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IcyBtcSwapOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IcyBtcSwapOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IcyBtcSwapOwnershipTransferred represents a OwnershipTransferred event raised by the IcyBtcSwap contract.
type IcyBtcSwapOwnershipTransferred struct {
	User     common.Address
	NewOwner common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed user, address indexed newOwner)
func (_IcyBtcSwap *IcyBtcSwapFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, user []common.Address, newOwner []common.Address) (*IcyBtcSwapOwnershipTransferredIterator, error) {

	var userRule []interface{}
	for _, userItem := range user {
		userRule = append(userRule, userItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _IcyBtcSwap.contract.FilterLogs(opts, "OwnershipTransferred", userRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapOwnershipTransferredIterator{contract: _IcyBtcSwap.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed user, address indexed newOwner)
func (_IcyBtcSwap *IcyBtcSwapFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *IcyBtcSwapOwnershipTransferred, user []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var userRule []interface{}
	for _, userItem := range user {
		userRule = append(userRule, userItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _IcyBtcSwap.contract.WatchLogs(opts, "OwnershipTransferred", userRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IcyBtcSwapOwnershipTransferred)
				if err := _IcyBtcSwap.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed user, address indexed newOwner)
func (_IcyBtcSwap *IcyBtcSwapFilterer) ParseOwnershipTransferred(log types.Log) (*IcyBtcSwapOwnershipTransferred, error) {
	event := new(IcyBtcSwapOwnershipTransferred)
	if err := _IcyBtcSwap.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IcyBtcSwapRevertIcyIterator is returned from FilterRevertIcy and is used to iterate over the raw logs and unpacked data for RevertIcy events raised by the IcyBtcSwap contract.
type IcyBtcSwapRevertIcyIterator struct {
	Event *IcyBtcSwapRevertIcy // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IcyBtcSwapRevertIcyIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IcyBtcSwapRevertIcy)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IcyBtcSwapRevertIcy)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IcyBtcSwapRevertIcyIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IcyBtcSwapRevertIcyIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IcyBtcSwapRevertIcy represents a RevertIcy event raised by the IcyBtcSwap contract.
type IcyBtcSwapRevertIcy struct {
	IcyAmount  *big.Int
	BtcAddress string
	BtcAmount  *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterRevertIcy is a free log retrieval operation binding the contract event 0xd65289b780c2a5756f2385450f37835d3af0fd779700af98d868c8f952e9acff.
//
// Solidity: event RevertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) FilterRevertIcy(opts *bind.FilterOpts) (*IcyBtcSwapRevertIcyIterator, error) {

	logs, sub, err := _IcyBtcSwap.contract.FilterLogs(opts, "RevertIcy")
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapRevertIcyIterator{contract: _IcyBtcSwap.contract, event: "RevertIcy", logs: logs, sub: sub}, nil
}

// WatchRevertIcy is a free log subscription operation binding the contract event 0xd65289b780c2a5756f2385450f37835d3af0fd779700af98d868c8f952e9acff.
//
// Solidity: event RevertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) WatchRevertIcy(opts *bind.WatchOpts, sink chan<- *IcyBtcSwapRevertIcy) (event.Subscription, error) {

	logs, sub, err := _IcyBtcSwap.contract.WatchLogs(opts, "RevertIcy")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IcyBtcSwapRevertIcy)
				if err := _IcyBtcSwap.contract.UnpackLog(event, "RevertIcy", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRevertIcy is a log parse operation binding the contract event 0xd65289b780c2a5756f2385450f37835d3af0fd779700af98d868c8f952e9acff.
//
// Solidity: event RevertIcy(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) ParseRevertIcy(log types.Log) (*IcyBtcSwapRevertIcy, error) {
	event := new(IcyBtcSwapRevertIcy)
	if err := _IcyBtcSwap.contract.UnpackLog(event, "RevertIcy", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IcyBtcSwapSetSignerIterator is returned from FilterSetSigner and is used to iterate over the raw logs and unpacked data for SetSigner events raised by the IcyBtcSwap contract.
type IcyBtcSwapSetSignerIterator struct {
	Event *IcyBtcSwapSetSigner // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IcyBtcSwapSetSignerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IcyBtcSwapSetSigner)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IcyBtcSwapSetSigner)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IcyBtcSwapSetSignerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IcyBtcSwapSetSignerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IcyBtcSwapSetSigner represents a SetSigner event raised by the IcyBtcSwap contract.
type IcyBtcSwapSetSigner struct {
	SignerAddress common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSetSigner is a free log retrieval operation binding the contract event 0xbb10aee7ef5a307b8097c6a7f2892b909ff1736fd24a6a5260640c185f7153b6.
//
// Solidity: event SetSigner(address signerAddress)
func (_IcyBtcSwap *IcyBtcSwapFilterer) FilterSetSigner(opts *bind.FilterOpts) (*IcyBtcSwapSetSignerIterator, error) {

	logs, sub, err := _IcyBtcSwap.contract.FilterLogs(opts, "SetSigner")
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapSetSignerIterator{contract: _IcyBtcSwap.contract, event: "SetSigner", logs: logs, sub: sub}, nil
}

// WatchSetSigner is a free log subscription operation binding the contract event 0xbb10aee7ef5a307b8097c6a7f2892b909ff1736fd24a6a5260640c185f7153b6.
//
// Solidity: event SetSigner(address signerAddress)
func (_IcyBtcSwap *IcyBtcSwapFilterer) WatchSetSigner(opts *bind.WatchOpts, sink chan<- *IcyBtcSwapSetSigner) (event.Subscription, error) {

	logs, sub, err := _IcyBtcSwap.contract.WatchLogs(opts, "SetSigner")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IcyBtcSwapSetSigner)
				if err := _IcyBtcSwap.contract.UnpackLog(event, "SetSigner", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSetSigner is a log parse operation binding the contract event 0xbb10aee7ef5a307b8097c6a7f2892b909ff1736fd24a6a5260640c185f7153b6.
//
// Solidity: event SetSigner(address signerAddress)
func (_IcyBtcSwap *IcyBtcSwapFilterer) ParseSetSigner(log types.Log) (*IcyBtcSwapSetSigner, error) {
	event := new(IcyBtcSwapSetSigner)
	if err := _IcyBtcSwap.contract.UnpackLog(event, "SetSigner", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IcyBtcSwapSwapIterator is returned from FilterSwap and is used to iterate over the raw logs and unpacked data for Swap events raised by the IcyBtcSwap contract.
type IcyBtcSwapSwapIterator struct {
	Event *IcyBtcSwapSwap // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IcyBtcSwapSwapIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IcyBtcSwapSwap)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IcyBtcSwapSwap)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IcyBtcSwapSwapIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IcyBtcSwapSwapIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IcyBtcSwapSwap represents a Swap event raised by the IcyBtcSwap contract.
type IcyBtcSwapSwap struct {
	IcyAmount  *big.Int
	BtcAddress string
	BtcAmount  *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterSwap is a free log retrieval operation binding the contract event 0x6a7e3add5ba4ffd84c70888b34c2abc4eb346e94dbd82a1ba6dd8e335c682063.
//
// Solidity: event Swap(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) FilterSwap(opts *bind.FilterOpts) (*IcyBtcSwapSwapIterator, error) {

	logs, sub, err := _IcyBtcSwap.contract.FilterLogs(opts, "Swap")
	if err != nil {
		return nil, err
	}
	return &IcyBtcSwapSwapIterator{contract: _IcyBtcSwap.contract, event: "Swap", logs: logs, sub: sub}, nil
}

// WatchSwap is a free log subscription operation binding the contract event 0x6a7e3add5ba4ffd84c70888b34c2abc4eb346e94dbd82a1ba6dd8e335c682063.
//
// Solidity: event Swap(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) WatchSwap(opts *bind.WatchOpts, sink chan<- *IcyBtcSwapSwap) (event.Subscription, error) {

	logs, sub, err := _IcyBtcSwap.contract.WatchLogs(opts, "Swap")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IcyBtcSwapSwap)
				if err := _IcyBtcSwap.contract.UnpackLog(event, "Swap", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSwap is a log parse operation binding the contract event 0x6a7e3add5ba4ffd84c70888b34c2abc4eb346e94dbd82a1ba6dd8e335c682063.
//
// Solidity: event Swap(uint256 icyAmount, string btcAddress, uint256 btcAmount)
func (_IcyBtcSwap *IcyBtcSwapFilterer) ParseSwap(log types.Log) (*IcyBtcSwapSwap, error) {
	event := new(IcyBtcSwapSwap)
	if err := _IcyBtcSwap.contract.UnpackLog(event, "Swap", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
