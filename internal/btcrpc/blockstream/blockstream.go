package blockstream

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type blockstream struct {
	baseURL string
	client  *http.Client
	logger  *logger.Logger
}

func New(cfg *config.AppConfig, logger *logger.Logger) IBlockStream {
	return &blockstream{
		baseURL: cfg.Bitcoin.BlockstreamAPIURL,
		client:  &http.Client{},
		logger:  logger,
	}
}

func (c *blockstream) BroadcastTx(txHex string) (string, error) {
	url := fmt.Sprintf("%s/tx", c.baseURL)
	payload := strings.NewReader(txHex)

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast transaction: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to broadcast transaction: %s", string(body))
	}

	return string(body), nil
}

// EstimateFee returns a map of confirmation target times (in blocks) to fee rates (in sat/vB)
// Example response:
//
//	{
//	  "1": 25.0,  // 25 sat/vB for next block
//	  "2": 20.0,  // 20 sat/vB for 2 blocks
//	  "3": 15.0,  // 15 sat/vB for 3 blocks
//	  "6": 10.0   // 10 sat/vB for 6 blocks
//	}
func (c *blockstream) EstimateFees() (fees map[string]float64, err error) {
	url := fmt.Sprintf("%s/fee-estimates", c.baseURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee estimates: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&fees); err != nil {
		return nil, fmt.Errorf("failed to parse fee estimates: %v", err)
	}

	return fees, nil
}

func (c *blockstream) GetUTXOs(address string) ([]UTXO, error) {
	url := fmt.Sprintf("%s/address/%s/utxo", c.baseURL, address)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %v", err)
	}
	defer resp.Body.Close()

	var utxos []UTXO
	if err := json.NewDecoder(resp.Body).Decode(&utxos); err != nil {
		return nil, fmt.Errorf("failed to parse UTXOs: %v", err)
	}

	return utxos, nil
}
