package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/vault"
)

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
}

type ApiServerConfig struct {
	AllowedOrigins string
	ApiKey         string
	AppEnv         string
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
	IcySwapSignerPrivateKey   string
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
	WalletWIF         string
	BlockstreamAPIURL string
	MaxTxFeeUSD       float64
	ServiceFeeRate    float64
	MinSatshiFee      int64
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
			AppEnv:         env,
			AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
			ApiKey:         os.Getenv("API_KEY"),
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
			WalletWIF:         btcWalletWIF,
			BlockstreamAPIURL: os.Getenv("BTC_BLOCKSTREAM_API_URL"),
			MaxTxFeeUSD:       envVarAsFloat("BTC_MAX_TX_FEE_USD", 1.0),
			ServiceFeeRate:    envVarAsFloat("BTC_SERVICE_FEE_PERCENTAGE", 0.01),
			MinSatshiFee:      envVarAsInt64("BTC_MIN_SATOSHI_FEE", 3000),
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
	}

	return config
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

func envVarAsBool(envName string) bool {
	valueStr := os.Getenv(envName)
	return valueStr == "true"
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
