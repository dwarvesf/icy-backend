// Package ratecheck holds black-box HTTP tests for the swap handler's
// server-side oracle-rate enforcement (CRIT-1). It lives in its own directory
// (not internal/handler/swap) purely because the pre-existing swap_test files
// in that package do not compile against the current interfaces, which would
// otherwise block any new test in that package from building.
package ratecheck

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func TestRateCheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Swap GenerateSignature Rate-Enforcement Suite")
}

// --- stub oracle: only the cached-balance methods are exercised ---

type stubOracle struct {
	circ *model.Web3BigInt
	btc  *model.Web3BigInt
	err  error
}

func (s *stubOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) { return s.circ, s.err }
func (s *stubOracle) GetCachedBTCSupply() (*model.Web3BigInt, error)     { return s.btc, s.err }

// remaining IOracle methods are unused by GenerateSignature
func (s *stubOracle) GetCirculatedICY() (*model.Web3BigInt, error)        { return s.circ, s.err }
func (s *stubOracle) GetBTCSupply() (*model.Web3BigInt, error)            { return s.btc, s.err }
func (s *stubOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error)       { return nil, nil }
func (s *stubOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) { return nil, nil }
func (s *stubOracle) GetCirculatedICYWithContext(context.Context) (*model.Web3BigInt, error) {
	return s.circ, s.err
}
func (s *stubOracle) GetBTCSupplyWithContext(context.Context) (*model.Web3BigInt, error) {
	return s.btc, s.err
}
func (s *stubOracle) RefreshCirculatedICYAsync() error            { return nil }
func (s *stubOracle) RefreshBTCSupplyAsync() error                { return nil }
func (s *stubOracle) ClearAllCaches() error                       { return nil }
func (s *stubOracle) GetCacheStatistics() *oracle.CacheStatistics { return nil }

// stubBaseRPC captures the btc amount actually handed to the signer.
type stubBaseRPC struct {
	gotICY *model.Web3BigInt
	gotBTC *model.Web3BigInt
}

func (s *stubBaseRPC) GenerateSignature(icy *model.Web3BigInt, _ string, btc *model.Web3BigInt, _ *big.Int, _ *big.Int) (string, error) {
	s.gotICY = icy
	s.gotBTC = btc
	return "deadbeef", nil
}
func (s *stubBaseRPC) Client() *ethclient.Client                      { return nil }
func (s *stubBaseRPC) GetContractAddress() common.Address             { return common.Address{} }
func (s *stubBaseRPC) ICYBalanceOf(string) (*model.Web3BigInt, error) { return nil, nil }
func (s *stubBaseRPC) ICYTotalSupply() (*model.Web3BigInt, error)     { return nil, nil }
func (s *stubBaseRPC) ICYTransferredTo(string, common.Address) (*big.Int, error) {
	return big.NewInt(0), nil
}
func (s *stubBaseRPC) GetTransactionsByAddress(string, string) ([]model.OnchainIcyTransaction, error) {
	return nil, nil
}
func (s *stubBaseRPC) Swap(*model.Web3BigInt, string, *model.Web3BigInt) (*types.Transaction, error) {
	return nil, nil
}

// stubBtcRPC: IsDust is the only method used by the handler here.
type stubBtcRPC struct{ dust bool }

func (s *stubBtcRPC) IsDust(string, int64) bool { return s.dust }
func (s *stubBtcRPC) Send(string, *model.Web3BigInt) (string, int64, error) {
	return "", 0, nil
}
func (s *stubBtcRPC) CurrentBalance() (*model.Web3BigInt, error) { return nil, nil }
func (s *stubBtcRPC) GetTransactionsByAddress(string, string) ([]model.OnchainBtcTransaction, error) {
	return nil, nil
}
func (s *stubBtcRPC) EstimateFees() (map[string]float64, error) { return nil, nil }
func (s *stubBtcRPC) GetSatoshiUSDPrice() (float64, error)      { return 0, nil }

var _ = Describe("POST /api/v1/swap/generate-signature server-side rate enforcement", func() {
	var (
		baseRPC *stubBaseRPC
		router  *gin.Engine
	)

	// Fixture: 1000 circulated ICY backed by 1 BTC => 100000 sat per ICY.
	const (
		circulatedICY = "1000000000000000000000" // 1000 ICY (18 decimals)
		btcSupplySat  = "100000000"              // 1 BTC in satoshi (8 decimals)
		icyTwoTokens  = "2000000000000000000"    // 2 ICY (18 decimals)
		oracleSat     = "200000"                 // 2 ICY * 100000 sat/ICY
	)

	newRouter := func(o oracle.IOracle, b *stubBaseRPC, btc *stubBtcRPC) *gin.Engine {
		gin.SetMode(gin.TestMode)
		appConfig := &config.AppConfig{
			MinIcySwapAmount: 1000,
			Bitcoin: config.BitcoinConfig{
				ServiceFeeRate: 0.01,
				MinSatshiFee:   546,
			},
		}
		h := swap.New(logger.New("test"), appConfig, o, b, btc, nil)
		r := gin.New()
		r.POST("/api/v1/swap/generate-signature", h.GenerateSignature)
		return r
	}

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/swap/generate-signature", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	BeforeEach(func() {
		baseRPC = &stubBaseRPC{}
		o := &stubOracle{
			circ: &model.Web3BigInt{Value: circulatedICY, Decimal: 18},
			btc:  &model.Web3BigInt{Value: btcSupplySat, Decimal: 8},
		}
		router = newRouter(o, baseRPC, &stubBtcRPC{dust: false})
	})

	It("(b) signs the oracle-derived btc_amount = icy_amount * rate", func() {
		w := post(`{"icy_amount":"` + icyTwoTokens + `","btc_address":"bc1qexample","btc_amount":"` + oracleSat + `"}`)

		Expect(w.Code).To(Equal(http.StatusOK))
		// The value actually handed to the signer is the server-derived amount.
		Expect(baseRPC.gotBTC).NotTo(BeNil())
		Expect(baseRPC.gotBTC.Value).To(Equal(oracleSat))
		// And it is echoed back in the response.
		Expect(w.Body.String()).To(ContainSubstring(`"btc_amount":"` + oracleSat + `"`))
	})

	It("(a) rejects an inflated client btc_amount (1000x the oracle amount)", func() {
		inflated := "200000000" // 1000x oracleSat
		w := post(`{"icy_amount":"` + icyTwoTokens + `","btc_address":"bc1qexample","btc_amount":"` + inflated + `"}`)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
		Expect(w.Body.String()).To(ContainSubstring("inflated"))
		// Nothing was signed.
		Expect(baseRPC.gotBTC).To(BeNil())
	})

	It("ignores a within-tolerance client btc_amount and still signs the oracle amount", func() {
		nearlyRight := "201000" // 0.5% over oracleSat: accepted, but must not be signed
		w := post(`{"icy_amount":"` + icyTwoTokens + `","btc_address":"bc1qexample","btc_amount":"` + nearlyRight + `"}`)

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(baseRPC.gotBTC.Value).To(Equal(oracleSat)) // server value, NOT 201000
	})

	It("returns a signature payload on success", func() {
		w := post(`{"icy_amount":"` + icyTwoTokens + `","btc_address":"bc1qexample","btc_amount":"` + oracleSat + `"}`)
		Expect(w.Code).To(Equal(http.StatusOK))

		var parsed map[string]json.RawMessage
		Expect(json.Unmarshal(w.Body.Bytes(), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("data"))
	})
})
