package blockstream

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
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
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create a new reader for each attempt since it gets consumed
		payload := strings.NewReader(txHex)

		req, err := http.NewRequest("POST", url, payload)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %v", err)
			c.logger.Error("[BroadcastTx][http.NewRequest]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		req.Header.Add("Content-Type", "text/plain")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to request broadcast transaction: %v", err)
			c.logger.Error("[BroadcastTx][client.Do]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %v", err)
			c.logger.Error("[BroadcastTx][io.ReadAll]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		if resp.StatusCode != 200 {
			// Check for minimum relay fee error
			bodyStr := string(body)

			// Regex to extract minimum fee from error message
			minFeeRegex := regexp.MustCompile(`sendrawtransaction RPC error -26: min relay fee not met, (\d+) < (\d+)`)
			matches := minFeeRegex.FindStringSubmatch(bodyStr)

			c.logger.Error("[BroadcastTx] broadcast error", map[string]string{
				"error":      bodyStr,
				"matches":    fmt.Sprintf("%v", matches),
				"match_len":  strconv.Itoa(len(matches)),
				"statusCode": strconv.Itoa(resp.StatusCode),
				"attempt":    strconv.Itoa(attempt),
			})

			if len(matches) == 3 {
				minFee, _ := strconv.ParseInt(matches[2], 10, 64)
				return "", &BroadcastTxError{
					Message:    fmt.Sprintf("status code: %v, failed to broadcast transaction: %s", resp.StatusCode, bodyStr),
					StatusCode: resp.StatusCode,
					MinFee:     minFee,
				}
			}

			lastErr = fmt.Errorf("status code: %v, failed to broadcast transaction: %s", resp.StatusCode, bodyStr)

			// Don't retry if it's a validation error (like insufficient funds)
			if resp.StatusCode == 400 {
				return "", lastErr
			}

			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		return string(body), nil
	}

	return "", lastErr
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
func (c *blockstream) EstimateFees() (map[string]float64, error) {
	url := fmt.Sprintf("%s/fee-estimates", c.baseURL)
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("failed to get fee estimates: %v", err)
			c.logger.Error("[EstimateFees][client.Get]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			c.logger.Error("[EstimateFees][client.Get]", map[string]string{
				"error":      lastErr.Error(),
				"statusCode": strconv.Itoa(resp.StatusCode),
				"attempt":    strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %v", err)
			c.logger.Error("[EstimateFees][io.ReadAll]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		var fees map[string]float64
		err = json.Unmarshal(body, &fees)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse fee estimates: %v", err)
			c.logger.Error("[EstimateFees][json.Unmarshal]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
				"body":    string(body),
			})
			continue
		}

		return fees, nil
	}

	return nil, lastErr
}

func (c *blockstream) GetUTXOs(address string) ([]UTXO, error) {
	url := fmt.Sprintf("%s/address/%s/utxo", c.baseURL, address)
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("failed to get UTXOs: %v", err)
			c.logger.Error("[GetUTXOs][client.Get]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			c.logger.Error("[GetUTXOs][client.Get]", map[string]string{
				"error":      lastErr.Error(),
				"statusCode": strconv.Itoa(resp.StatusCode),
				"attempt":    strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %v", err)
			c.logger.Error("[GetUTXOs][io.ReadAll]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		var utxos []UTXO
		err = json.Unmarshal(body, &utxos)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse UTXOs: %v", err)
			c.logger.Error("[GetUTXOs][json.Unmarshal]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
				"body":    string(body),
			})
			continue
		}

		return utxos, nil
	}

	return nil, lastErr
}

func (c *blockstream) GetBTCBalance(address string) (*model.Web3BigInt, error) {
	url := fmt.Sprintf("%s/address/%s", c.baseURL, address)
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.client.Get(url)
		if err != nil {
			lastErr = errors.Wrap(err, "failed to fetch BTC balance")
			c.logger.Error("[GetBTCBalance][client.Get]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			continue
		}

		// Ensure response body is closed properly
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			c.logger.Error("[GetBTCBalance][client.Get]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = errors.Wrap(err, "failed to read response body")
			c.logger.Error("[GetBTCBalance][io.ReadAll]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		var response GetBalanceResponse
		err = json.Unmarshal(body, &response)
		if err != nil {
			lastErr = errors.Wrap(err, "failed to parse JSON response")
			c.logger.Error("[GetBTCBalance][json.Unmarshal]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		// Correct balance calculation
		balanceSats := response.ChainStats.FundedTxoSum - response.ChainStats.SpentTxoSum
		return &model.Web3BigInt{
			Value:   strconv.FormatInt(int64(balanceSats), 10),
			Decimal: consts.BTC_DECIMALS, // BTC has 8 decimal places
		}, nil
	}

	return nil, lastErr
}

func (c *blockstream) GetTransactionsByAddress(address string, fromTxID string) ([]Transaction, error) {
	var url string
	if fromTxID == "" {
		url = fmt.Sprintf("%s/address/%s/txs", c.baseURL, address)
	} else {
		url = fmt.Sprintf("%s/address/%s/txs/chain/%s", c.baseURL, address, fromTxID)
	}

	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("failed to get transactions: %v", err)
			c.logger.Error("[GetTransactionsByAddress][client.Get]", map[string]string{
				"error":   err.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			c.logger.Error("[GetTransactionsByAddress][client.Get]", map[string]string{
				"error":      lastErr.Error(),
				"statusCode": strconv.Itoa(resp.StatusCode),
				"attempt":    strconv.Itoa(attempt),
			})
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %v", err)
			c.logger.Error("[GetTransactionsByAddress][io.ReadAll]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
			})
			continue
		}

		var txs []Transaction
		err = json.Unmarshal(body, &txs)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse transactions: %v", err)
			c.logger.Error("[GetTransactionsByAddress][json.Unmarshal]", map[string]string{
				"error":   lastErr.Error(),
				"attempt": strconv.Itoa(attempt),
				"body":    string(body),
			})
			continue
		}
		return txs, nil
	}
	return nil, lastErr
}
