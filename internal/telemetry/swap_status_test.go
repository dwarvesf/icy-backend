package telemetry_test

import (
	"context"
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// --- fakeSwapRequestStore -------------------------------------------------
//
// An in-memory double for swaprequest.IStore that mirrors the real store's
// `WHERE status = 'pending'` filter in FindPendingSwapRequests. This is what
// makes the regression test meaningful: calling ProcessSwapRequests twice
// against the same fake behaves like two real cron ticks against the same
// Postgres row.

type fakeSwapRequestStore struct {
	mu       sync.Mutex
	requests map[string]*model.SwapRequest // keyed by IcyTx
}

func newFakeSwapRequestStore(reqs ...*model.SwapRequest) *fakeSwapRequestStore {
	m := map[string]*model.SwapRequest{}
	for _, r := range reqs {
		m[r.IcyTx] = r
	}
	return &fakeSwapRequestStore{requests: m}
}

func (f *fakeSwapRequestStore) Create(tx *gorm.DB, swapRequest *model.SwapRequest) (*model.SwapRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests[swapRequest.IcyTx] = swapRequest
	return swapRequest, nil
}

func (f *fakeSwapRequestStore) GetByIcyTx(tx *gorm.DB, icyTx string) (*model.SwapRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[icyTx]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return r, nil
}

func (f *fakeSwapRequestStore) FindPendingSwapRequests(tx *gorm.DB) ([]model.SwapRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.SwapRequest
	for _, r := range f.requests {
		if r.Status == model.SwapRequestStatusPending {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeSwapRequestStore) UpdateStatus(tx *gorm.DB, icyTx, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[icyTx]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	r.Status = model.SwapRequestStatus(status)
	return nil
}

func (f *fakeSwapRequestStore) statusOf(icyTx string) model.SwapRequestStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[icyTx].Status
}

// --- stubOnchainIcyTxStore ------------------------------------------------
// ProcessSwapRequests only needs GetByTransactionHash to succeed so the loop
// reaches the swap step; the other methods are unused by this code path.

type stubOnchainIcyTxStore struct{}

func (s *stubOnchainIcyTxStore) Create(db *gorm.DB, t *model.OnchainIcyTransaction) (*model.OnchainIcyTransaction, error) {
	return t, nil
}

func (s *stubOnchainIcyTxStore) GetLatestTransaction(db *gorm.DB) (*model.OnchainIcyTransaction, error) {
	return nil, nil
}

func (s *stubOnchainIcyTxStore) GetByTransactionHash(db *gorm.DB, txHash string) (*model.OnchainIcyTransaction, error) {
	return &model.OnchainIcyTransaction{TransactionHash: txHash}, nil
}

// --- stubBaseRPC -----------------------------------------------------------
// Records every Swap() invocation (and the nonce it was called with) so
// tests can assert both "called at most once" and "same nonce on retry".

type stubBaseRPC struct {
	mu      sync.Mutex
	nonces  []*big.Int
	swapErr error
}

func (s *stubBaseRPC) Client() *ethclient.Client          { return nil }
func (s *stubBaseRPC) GetContractAddress() common.Address { return common.Address{} }

func (s *stubBaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) { return nil, nil }
func (s *stubBaseRPC) ICYTotalSupply() (*model.Web3BigInt, error)             { return nil, nil }

func (s *stubBaseRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error) {
	return nil, nil
}

func (s *stubBaseRPC) GenerateSignature(icyAmount *model.Web3BigInt, btcAddress string, btcAmount *model.Web3BigInt, nonce *big.Int, deadline *big.Int) (string, error) {
	return "", nil
}

func (s *stubBaseRPC) Swap(icyAmount *model.Web3BigInt, btcAddress string, btcAmount *model.Web3BigInt, nonce *big.Int) (*types.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nonces = append(s.nonces, new(big.Int).Set(nonce))
	if s.swapErr != nil {
		return nil, s.swapErr
	}
	return &types.Transaction{}, nil
}

func (s *stubBaseRPC) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.nonces)
}

// --- stubOracle --------------------------------------------------------
// Only GetRealtimeICYBTC is exercised by ProcessSwapRequests; every other
// method just needs to exist to satisfy oracle.IOracle.

type stubOracle struct {
	price *model.Web3BigInt
}

func (s *stubOracle) GetCirculatedICY() (*model.Web3BigInt, error)  { return nil, nil }
func (s *stubOracle) GetBTCSupply() (*model.Web3BigInt, error)      { return nil, nil }
func (s *stubOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) { return s.price, nil }
func (s *stubOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	return s.price, nil
}
func (s *stubOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) { return nil, nil }
func (s *stubOracle) GetCachedBTCSupply() (*model.Web3BigInt, error)     { return nil, nil }
func (s *stubOracle) GetCirculatedICYWithContext(ctx context.Context) (*model.Web3BigInt, error) {
	return nil, nil
}
func (s *stubOracle) GetBTCSupplyWithContext(ctx context.Context) (*model.Web3BigInt, error) {
	return nil, nil
}
func (s *stubOracle) RefreshCirculatedICYAsync() error            { return nil }
func (s *stubOracle) RefreshBTCSupplyAsync() error                { return nil }
func (s *stubOracle) ClearAllCaches() error                       { return nil }
func (s *stubOracle) GetCacheStatistics() *oracle.CacheStatistics { return nil }

// --- CRIT-2 regression specs ------------------------------------------------

var _ = Describe("ProcessSwapRequests status tracking (CRIT-2 regression)", func() {
	var (
		testLogger *logger.Logger
		appConfig  *config.AppConfig
		oracleSvc  *stubOracle
	)

	BeforeEach(func() {
		testLogger = logger.New(environments.Test)
		appConfig = &config.AppConfig{}
		oracleSvc = &stubOracle{price: &model.Web3BigInt{Value: "1000000000000000", Decimal: 18}}
	})

	newService := func(swapReqStore *fakeSwapRequestStore, baseRPC *stubBaseRPC) *telemetry.Telemetry {
		st := &store.Store{
			OnchainIcyTransaction: &stubOnchainIcyTxStore{},
			SwapRequest:           swapReqStore,
		}
		return telemetry.New(nil, st, appConfig, testLogger, nil, baseRPC, oracleSvc)
	}

	It("settles a pending swap request at most once across repeated cron ticks", func() {
		req := &model.SwapRequest{
			ICYAmount:  "5000000000000000000000",
			BTCAddress: "tb1qtest123",
			IcyTx:      "0xswap-once",
			Status:     model.SwapRequestStatusPending,
		}
		swapReqStore := newFakeSwapRequestStore(req)
		baseRPC := &stubBaseRPC{}
		svc := newService(swapReqStore, baseRPC)

		// Tick 1: the request is pending, so it gets swapped and settled.
		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(1))
		Expect(swapReqStore.statusOf(req.IcyTx)).To(Equal(model.SwapRequestStatusCompleted))

		// Tick 2: this is the ~2-minute cron re-run that caused CRIT-2. The
		// request is no longer 'pending', so FindPendingSwapRequests must not
		// return it, and baseRPC.Swap must not be called again.
		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(1), "a completed swap request must not be re-swapped on a later tick")
	})

	It("never re-processes a request that is already 'processing' or 'completed'", func() {
		processing := &model.SwapRequest{ICYAmount: "1", BTCAddress: "a", IcyTx: "0xprocessing", Status: model.SwapRequestStatusProcessing}
		completed := &model.SwapRequest{ICYAmount: "1", BTCAddress: "b", IcyTx: "0xcompleted", Status: model.SwapRequestStatusCompleted}
		failed := &model.SwapRequest{ICYAmount: "1", BTCAddress: "c", IcyTx: "0xfailed", Status: model.SwapRequestStatusFailed}
		swapReqStore := newFakeSwapRequestStore(processing, completed, failed)
		baseRPC := &stubBaseRPC{}
		svc := newService(swapReqStore, baseRPC)

		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(0), "only 'pending' requests may ever reach baseRPC.Swap")
	})

	It("marks a failed swap 'failed' (terminal), not left re-triable as 'pending'", func() {
		req := &model.SwapRequest{
			ICYAmount:  "5000000000000000000000",
			BTCAddress: "tb1qtest123",
			IcyTx:      "0xswap-fails",
			Status:     model.SwapRequestStatusPending,
		}
		swapReqStore := newFakeSwapRequestStore(req)
		baseRPC := &stubBaseRPC{swapErr: errors.New("rpc: swap reverted")}
		svc := newService(swapReqStore, baseRPC)

		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(1))
		Expect(swapReqStore.statusOf(req.IcyTx)).To(Equal(model.SwapRequestStatusFailed))

		// A second tick must still not re-swap a failed (non-pending) request.
		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(1))
	})

	It("reuses the same nonce for the same request across attempts", func() {
		req := &model.SwapRequest{
			ICYAmount:  "5000000000000000000000",
			BTCAddress: "tb1qtest123",
			IcyTx:      "0xsame-request",
			Status:     model.SwapRequestStatusPending,
		}
		swapReqStore := newFakeSwapRequestStore(req)
		baseRPC := &stubBaseRPC{swapErr: errors.New("transient network blip")}
		svc := newService(swapReqStore, baseRPC)

		// First attempt fails -> status becomes 'failed'.
		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(1))
		Expect(swapReqStore.statusOf(req.IcyTx)).To(Equal(model.SwapRequestStatusFailed))

		// An operator manually re-queues the request for a legitimate retry.
		swapReqStore.mu.Lock()
		swapReqStore.requests[req.IcyTx].Status = model.SwapRequestStatusPending
		swapReqStore.mu.Unlock()
		baseRPC.swapErr = nil

		Expect(svc.ProcessSwapRequests()).To(Succeed())
		Expect(baseRPC.callCount()).To(Equal(2))
		Expect(baseRPC.nonces[0].Cmp(baseRPC.nonces[1])).To(Equal(0),
			"retrying the same request must reuse its nonce so the on-chain replay guard can dedupe it")
	})
})
