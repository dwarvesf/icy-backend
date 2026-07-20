package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/vault"
)

// envInt64 reads an int64 env var, falling back to def when unset or unparseable.
func envInt64(key string, def int64) int64 {
	v, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil {
		return def
	}
	return v
}

// envBool reads a bool env var via ParseBool, so "1", "TRUE", "True" and "t"
// all work. A bare `== "true"` comparison silently reads every one of those as
// false, which for a security toggle means an operator turns enforcement on and
// it stays off.
func envBool(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return v
}

type AppConfig struct {
	Environment      environments.Environment
	ApiServer        ApiServerConfig
	Postgres         DBConnection
	Bitcoin          BitcoinConfig
	Blockchain       BlockchainConfig
	VaultConfig      VaultConfig
	IndexInterval    string
	MinIcySwapAmount float64
	MochiConfig      MochiConfig
	UptimeWebhooks   UptimeWebhookConfig
	// SwapPayoutWebhookURL is a Discord webhook fired whenever a BTC payout
	// reaches a terminal settlement state (completed / failed /
	// needs_reconcile). Detection only; empty means the notification is
	// skipped (see internal/telemetry/btc.go).
	SwapPayoutWebhookURL string
}

type UptimeWebhookConfig struct {
	IndexBtcTransactionURL           string
	IndexIcyTransactionURL           string
	IndexIcySwapTransactionURL       string
	ProcessPendingBtcTransactionsURL string
}

type ApiServerConfig struct {
	AllowedOrigins string
	ApiKey         string
	AppEnv         string
	// RequireWalletAuth gates ENFORCEMENT of the EIP-712 wallet signature on
	// /swap/generate-signature. A supplied signature is always verified; this
	// only decides whether one is mandatory, so the frontend can deploy first
	// and the flag flips afterwards. Env REQUIRE_WALLET_AUTH.
	RequireWalletAuth bool
	// WalletAuthChainID is the chain id in the EIP-712 domain. It must match the
	// frontend's domain exactly or the recovered address differs and every
	// signature looks invalid. Base mainnet is 8453. Env WALLET_AUTH_CHAIN_ID.
	WalletAuthChainID int64
	// TrustedProxies is a comma-separated CIDR list of hops in front of this
	// service. Empty (the default) means X-Forwarded-For is IGNORED and the
	// socket peer is the client IP. Without this gin trusts all proxies, which
	// makes every per-IP rate limit bypassable with a header.
	// Env TRUSTED_PROXIES.
	TrustedProxies string
}

type MochiConfig struct {
	MochiPayAPIURL string
}

type BlockchainConfig struct {
	BaseRPCEndpoint           string   // Primary endpoint (for backward compatibility)
	BaseRPCEndpoints          []string // Multiple endpoints for high availability
	ICYContractAddr           string
	ICYSwapContractAddr       string
	InitialICYSwapBlockNumber int
	BTCTreasuryAddress        string
	InitialICYTransactionHash string
	// IcySwapSignerPrivateKey is, despite the "signer" in its name, the HOLDER /
	// GAS / TX-SENDING wallet: baserpc uses it to build the on-chain transactor
	// (Approve + Swap), so it pays gas and is the ICY holder. Env
	// BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY (vault-transit-decrypted in prod).
	IcySwapSignerPrivateKey string
	// SwapSignerPK is the DEDICATED EIP-712 payout-authorization signer key,
	// isolated from the holder/gas wallet above (SG-06). It is used ONLY by
	// baserpc.GenerateSignature's crypto signing path and is NEVER wired into a
	// transactor, so an address provisioned here holds no funds and cannot send a
	// tx or pay gas: a leak of it cannot move value, only forge payout
	// signatures the on-chain contract still bounds. If empty, baserpc falls back
	// to IcySwapSignerPrivateKey so nothing breaks pre-provision. Env SWAP_SIGNER_PK
	// (or a vault ref); the treasurer provisions a DISTINCT key in step 08.
	SwapSignerPK string
	// MinSwapConfirmations is how many block confirmations a Base ICY-swap event
	// must have before it triggers a BTC payout. It is the reorg gate: an event
	// mined but not yet buried this deep is deferred (not recorded, not paid), so
	// a shallow reorg that unwinds the event cannot leave the treasury having paid
	// BTC for a swap that no longer exists. Inclusion block counts as the first
	// confirmation. Env BLOCKCHAIN_MIN_SWAP_CONFIRMATIONS, default 6.
	MinSwapConfirmations int64
}

type DBConnection struct {
	Host string
	Port string
	User string
	Name string
	Pass string

	SSLMode string
}

type BitcoinConfig struct {
	WalletWIF          string
	BlockstreamAPIURL  string   // Primary endpoint (for backward compatibility)
	BlockstreamAPIURLs []string // Multiple endpoints for high availability
	MaxTxFeeUSD        float64
	ServiceFeeRate     float64
	MinSatshiFee       int64
	// MinBtcConfirmations is how many on-chain confirmations an outgoing BTC
	// payout must reach before it is marked completed (confirm-before-complete).
	// A just-broadcast tx sits in the intermediate "broadcasted" state until it
	// crosses this threshold. Floored at 1 at the point of use (a payout can never
	// complete before it is at least mined). Env BTC_MIN_CONFIRMATIONS, default 1.
	MinBtcConfirmations int64
	// StuckTxTimeoutSeconds bounds how long a broadcasted-but-unconfirmed payout
	// may sit before it is flagged stuck and routed to needs_reconcile for a
	// manual fee-bump/replace (detect-and-reconcile; no auto-RBF, see
	// docs/verification/confirmation-depth.md). Env BTC_STUCK_TX_TIMEOUT_SECONDS,
	// default 10800 (3h).
	StuckTxTimeoutSeconds int64
	// MaxPayoutSatoshi is the PER-PAYOUT hard cap (in satoshi) on a single BTC
	// settlement. A payout whose sendable amount (subtotal - service fee) exceeds
	// this is REFUSED in code before any broadcast: the settlement orchestrator
	// marks the row failed (terminal; a fixed-size payout can never shrink under
	// the cap) and btcrpc.Send carries the same limit as a defence-in-depth
	// backstop for any direct caller. 0 disables the cap. Env BTC_MAX_PAYOUT_SATOSHI,
	// default 5_000_000 (0.05 BTC) as a SAFE placeholder; confirm the real prod
	// value with the treasurer before the BTC deposit (SG-06 / step 08).
	MaxPayoutSatoshi int64
	// MaxDailyPayoutSatoshi is the ROLLING 24h cap (in satoshi) on total BTC
	// outflow. Before each payout the orchestrator sums the sendable amount of the
	// last 24h of sent rows (broadcasted / completed / needs_reconcile); a payout
	// that would push that running total over this cap is REFUSED and routed to
	// needs_reconcile (treasurer review; never left pending to auto-retry forever).
	// 0 disables the cap. Env BTC_MAX_DAILY_PAYOUT_SATOSHI, default 25_000_000
	// (0.25 BTC) as a SAFE placeholder; confirm the real prod value with the
	// treasurer before the BTC deposit (SG-06 / step 08).
	MaxDailyPayoutSatoshi int64
}

type VaultConfig struct {
	Addr         string
	RoleName     string
	KVSecretPath string
}

func New() *AppConfig {
	env := os.Getenv("APP_ENV")

	// this will load .env file (env from travel-exp repo)
	// this will not override env variables if they already exist
	godotenv.Load(".env." + env)

	// Initialize variables
	btcWalletWIF := os.Getenv("BTC_WALLET_WIF")
	signerPrivateKey := os.Getenv("BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY")
	var err error
	var vc *vault.VaultClient

	// Initialize config with default values from environment variables
	config := &AppConfig{
		ApiServer: ApiServerConfig{
			AppEnv:            env,
			AllowedOrigins:    os.Getenv("ALLOWED_ORIGINS"),
			ApiKey:            os.Getenv("API_KEY"),
			RequireWalletAuth: envBool("REQUIRE_WALLET_AUTH", false),
			WalletAuthChainID: envInt64("WALLET_AUTH_CHAIN_ID", 8453),
			TrustedProxies:    os.Getenv("TRUSTED_PROXIES"),
		},
		Postgres: DBConnection{
			Host:    os.Getenv("DB_HOST"),
			Port:    os.Getenv("DB_PORT"),
			User:    os.Getenv("DB_USER"),
			Name:    os.Getenv("DB_NAME"),
			Pass:    os.Getenv("DB_PASS"),
			SSLMode: os.Getenv("DB_SSL_MODE"),
		},
		Bitcoin: BitcoinConfig{
			WalletWIF:             btcWalletWIF,
			BlockstreamAPIURL:     os.Getenv("BTC_BLOCKSTREAM_API_URL"),
			BlockstreamAPIURLs:    parseEndpoints(os.Getenv("BTC_BLOCKSTREAM_API_URLS"), os.Getenv("BTC_BLOCKSTREAM_API_URL")),
			MaxTxFeeUSD:           envVarAsFloat("BTC_MAX_TX_FEE_USD", 1.0),
			ServiceFeeRate:        envVarAsFloat("BTC_SERVICE_FEE_PERCENTAGE", 0.01),
			MinSatshiFee:          envVarAsInt64("BTC_MIN_SATOSHI_FEE", 3000),
			MinBtcConfirmations:   envVarAsInt64("BTC_MIN_CONFIRMATIONS", 1),
			StuckTxTimeoutSeconds: envVarAsInt64("BTC_STUCK_TX_TIMEOUT_SECONDS", 10800),
			MaxPayoutSatoshi:      envVarAsInt64("BTC_MAX_PAYOUT_SATOSHI", 5_000_000),
			MaxDailyPayoutSatoshi: envVarAsInt64("BTC_MAX_DAILY_PAYOUT_SATOSHI", 25_000_000),
		},
		Blockchain: BlockchainConfig{
			BaseRPCEndpoint:           os.Getenv("BLOCKCHAIN_BASE_RPC_ENDPOINT"),
			BaseRPCEndpoints:          parseEndpoints(os.Getenv("BLOCKCHAIN_BASE_RPC_ENDPOINTS"), os.Getenv("BLOCKCHAIN_BASE_RPC_ENDPOINT")),
			ICYContractAddr:           os.Getenv("BLOCKCHAIN_ICY_CONTRACT_ADDR"),
			ICYSwapContractAddr:       os.Getenv("BLOCKCHAIN_ICY_SWAP_CONTRACT_ADDR"),
			InitialICYSwapBlockNumber: envVarAtoi("BLOCKCHAIN_INITIAL_ICY_SWAP_BLOCK_NUMBER"),
			BTCTreasuryAddress:        os.Getenv("BLOCKCHAIN_BTC_TREASURY_ADDRESS"),
			InitialICYTransactionHash: os.Getenv("BLOCKCHAIN_INITIAL_ICY_TRANSACTION_HASH"),
			IcySwapSignerPrivateKey:   signerPrivateKey,
			SwapSignerPK:              os.Getenv("SWAP_SIGNER_PK"),
			MinSwapConfirmations:      envVarAsInt64("BLOCKCHAIN_MIN_SWAP_CONFIRMATIONS", 6),
		},
		IndexInterval:    os.Getenv("INDEX_INTERVAL"),
		MinIcySwapAmount: envVarAsFloat("MIN_ICY_SWAP_AMOUNT", 2000000000000000000),
		VaultConfig: VaultConfig{
			Addr:         os.Getenv("VAULT_ADDR"),
			RoleName:     os.Getenv("VAULT_ROLE_NAME"),
			KVSecretPath: os.Getenv("VAULT_KV_SECRET_PATH"),
		},
		MochiConfig: MochiConfig{
			MochiPayAPIURL: os.Getenv("MOCHI_PAY_API_URL"),
		},
		UptimeWebhooks: UptimeWebhookConfig{
			IndexBtcTransactionURL:           os.Getenv("INDEX_BTC_TRANSACTION_UPTIME_WEBHOOK_URL"),
			IndexIcyTransactionURL:           os.Getenv("INDEX_ICY_TRANSACTION_UPTIME_WEBHOOK_URL"),
			IndexIcySwapTransactionURL:       os.Getenv("INDEX_ICY_SWAP_TRANSACTION_UPTIME_WEBHOOK_URL"),
			ProcessPendingBtcTransactionsURL: os.Getenv("PROCESS_PENDING_BTC_TRANSACTIONS_UPTIME_WEBHOOK_URL"),
		},
		SwapPayoutWebhookURL: os.Getenv("SWAP_PAYOUT_WEBHOOK_URL"),
	}

	// If environment is not local, use vault for configuration
	if env != "" && env != "local" {
		vc = vault.New(os.Getenv("VAULT_ADDR"), os.Getenv("VAULT_KV_SECRET_PATH"), os.Getenv("VAULT_ROLE_NAME"))

		// Decrypt sensitive data using transit engine
		transitKeyPrefix, _ := vc.GetKV("VAULT_TRANSIT_KEY_PREFIX")
		ciphertext, _ := vc.GetKV("BTC_WALLET_WIF")
		btcWalletWIF, err = vc.DecryptData(fmt.Sprintf("%s-BTC_WALLET_WIF", transitKeyPrefix), ciphertext)
		if err != nil {
			panic(err)
		}

		ciphertext, _ = vc.GetKV("BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY")
		signerPrivateKey, err = vc.DecryptData(fmt.Sprintf("%s-BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY", transitKeyPrefix), ciphertext)
		if err != nil {
			panic(err)
		}

		// Update the decrypted values
		config.Bitcoin.WalletWIF = btcWalletWIF
		config.Blockchain.IcySwapSignerPrivateKey = signerPrivateKey

		// Optional DEDICATED EIP-712 signer key (SG-06). Provisioned in step 08 as
		// a transit-encrypted KV under SWAP_SIGNER_PK. Non-fatal by design: if the
		// KV is absent or fails to decrypt, SwapSignerPK stays empty and baserpc
		// falls back to the holder key, so an un-provisioned prod still boots.
		if signerPKCipher, _ := vc.GetKV("SWAP_SIGNER_PK"); signerPKCipher != "" {
			if swapSignerPK, derr := vc.DecryptData(fmt.Sprintf("%s-SWAP_SIGNER_PK", transitKeyPrefix), signerPKCipher); derr == nil {
				config.Blockchain.SwapSignerPK = swapSignerPK
			}
		}

		// Read other config values from vault
		// API Server config
		config.ApiServer.AllowedOrigins, _ = vc.GetKV("ALLOWED_ORIGINS")
		config.ApiServer.ApiKey, _ = vc.GetKV("API_KEY")

		// Postgres config
		config.Postgres.Host, _ = vc.GetKV("DB_HOST")
		config.Postgres.Port, _ = vc.GetKV("DB_PORT")
		config.Postgres.User, _ = vc.GetKV("DB_USER")
		config.Postgres.Name, _ = vc.GetKV("DB_NAME")
		config.Postgres.Pass, _ = vc.GetKV("DB_PASS")
		config.Postgres.SSLMode, _ = vc.GetKV("DB_SSL_MODE")

		// Bitcoin config
		config.Bitcoin.BlockstreamAPIURL, _ = vc.GetKV("BTC_BLOCKSTREAM_API_URL")

		btcEndpointsStr, _ := vc.GetKV("BTC_BLOCKSTREAM_API_URLS")
		config.Bitcoin.BlockstreamAPIURLs = parseEndpoints(btcEndpointsStr, config.Bitcoin.BlockstreamAPIURL)

		maxTxFeeUSD, _ := vc.GetKV("BTC_MAX_TX_FEE_USD")
		if maxTxFeeUSD != "" {
			config.Bitcoin.MaxTxFeeUSD, _ = strconv.ParseFloat(maxTxFeeUSD, 64)
		}

		serviceFeeRate, _ := vc.GetKV("BTC_SERVICE_FEE_PERCENTAGE")
		if serviceFeeRate != "" {
			config.Bitcoin.ServiceFeeRate, _ = strconv.ParseFloat(serviceFeeRate, 64)
		}

		minSatoshiFee, _ := vc.GetKV("BTC_MIN_SATOSHI_FEE")
		if minSatoshiFee != "" {
			config.Bitcoin.MinSatshiFee, _ = strconv.ParseInt(minSatoshiFee, 10, 64)
		}

		// Blockchain config
		config.Blockchain.BaseRPCEndpoint, _ = vc.GetKV("BLOCKCHAIN_BASE_RPC_ENDPOINT")

		endpointsStr, _ := vc.GetKV("BLOCKCHAIN_BASE_RPC_ENDPOINTS")
		config.Blockchain.BaseRPCEndpoints = parseEndpoints(endpointsStr, config.Blockchain.BaseRPCEndpoint)

		config.Blockchain.ICYContractAddr, _ = vc.GetKV("BLOCKCHAIN_ICY_CONTRACT_ADDR")
		config.Blockchain.ICYSwapContractAddr, _ = vc.GetKV("BLOCKCHAIN_ICY_SWAP_CONTRACT_ADDR")

		initialBlockNumber, _ := vc.GetKV("BLOCKCHAIN_INITIAL_ICY_SWAP_BLOCK_NUMBER")
		if initialBlockNumber != "" {
			config.Blockchain.InitialICYSwapBlockNumber, _ = strconv.Atoi(initialBlockNumber)
		}

		config.Blockchain.BTCTreasuryAddress, _ = vc.GetKV("BLOCKCHAIN_BTC_TREASURY_ADDRESS")
		config.Blockchain.InitialICYTransactionHash, _ = vc.GetKV("BLOCKCHAIN_INITIAL_ICY_TRANSACTION_HASH")

		// Other config
		config.IndexInterval, _ = vc.GetKV("INDEX_INTERVAL")

		minIcySwapAmount, _ := vc.GetKV("MIN_ICY_SWAP_AMOUNT")
		if minIcySwapAmount != "" {
			config.MinIcySwapAmount, _ = strconv.ParseFloat(minIcySwapAmount, 64)
		}

		// Mochi config
		config.MochiConfig.MochiPayAPIURL, _ = vc.GetKV("MOCHI_PAY_API_URL")

		// Uptime webhook config
		config.UptimeWebhooks.IndexBtcTransactionURL, _ = vc.GetKV("INDEX_BTC_TRANSACTION_UPTIME_WEBHOOK_URL")
		config.UptimeWebhooks.IndexIcyTransactionURL, _ = vc.GetKV("INDEX_ICY_TRANSACTION_UPTIME_WEBHOOK_URL")
		config.UptimeWebhooks.IndexIcySwapTransactionURL, _ = vc.GetKV("INDEX_ICY_SWAP_TRANSACTION_UPTIME_WEBHOOK_URL")
		config.UptimeWebhooks.ProcessPendingBtcTransactionsURL, _ = vc.GetKV("PROCESS_PENDING_BTC_TRANSACTIONS_UPTIME_WEBHOOK_URL")

		// Swap payout settlement webhook (Discord, SG-07)
		config.SwapPayoutWebhookURL, _ = vc.GetKV("SWAP_PAYOUT_WEBHOOK_URL")
	}

	// Fail closed at startup: refuse to boot a production server whose api-key gate
	// (the sole auth on the treasury-draining generate-signature endpoint) is empty.
	if err := validateSecurityConfig(config); err != nil {
		log.Fatalf("[config.New] insecure configuration: %v", err)
	}

	return config
}

// validateSecurityConfig enforces the security invariants a running server must
// satisfy. It returns an error (rather than exiting) so it is unit-testable; the
// caller in New() treats a non-nil error as fatal.
//
// Invariant: in a production environment the api-key middleware is the only auth
// in front of /swap/generate-signature, so an empty configured API key would leave
// that endpoint unauthenticated. Both "prod" and "production" are recognized.
func validateSecurityConfig(cfg *AppConfig) error {
	env := cfg.ApiServer.AppEnv
	if (env == "prod" || env == "production") && cfg.ApiServer.ApiKey == "" {
		return fmt.Errorf("API_KEY must be set when APP_ENV=%q; refusing to start with an unauthenticated generate-signature endpoint", env)
	}
	return nil
}

func envVarAsFloat(envName string, defaultValue float64) float64 {
	valueStr := os.Getenv(envName)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

func envVarAtoi(envName string) int {
	valueStr := os.Getenv(envName)
	if valueStr == "" {
		return 0
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(err)
	}

	return value
}

func envVarAsInt64(envName string, defaultValue int64) int64 {
	valueStr := os.Getenv(envName)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

// parseEndpoints parses a comma-separated list of endpoints and ensures the primary endpoint is included
func parseEndpoints(endpointsStr string, primaryEndpoint string) []string {
	if endpointsStr == "" {
		// If no endpoints are specified, use the primary endpoint
		if primaryEndpoint != "" {
			return []string{primaryEndpoint}
		}
		return []string{}
	}

	// Split the comma-separated list
	endpoints := strings.Split(endpointsStr, ",")

	// Trim whitespace from each endpoint
	for i := range endpoints {
		endpoints[i] = strings.TrimSpace(endpoints[i])
	}

	// Check if the primary endpoint is already in the list
	primaryIncluded := false
	for _, endpoint := range endpoints {
		if endpoint == primaryEndpoint {
			primaryIncluded = true
			break
		}
	}

	// If the primary endpoint is not in the list and it's not empty, add it to the beginning
	if !primaryIncluded && primaryEndpoint != "" {
		endpoints = append([]string{primaryEndpoint}, endpoints...)
	}

	return endpoints
}
