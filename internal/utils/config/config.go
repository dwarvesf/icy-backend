package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
)

type AppConfig struct {
	Environment      environments.Environment
	ApiServer        ApiServerConfig
	Postgres         DBConnection
	Bitcoin          BitcoinConfig
	Blockchain       BlockchainConfig
	IndexInterval    string
	MinIcySwapAmount float64
}

type ApiServerConfig struct {
	AllowedOrigins string
	ApiKey         string
	AppEnv         string
}

type BlockchainConfig struct {
	BaseRPCEndpoint           string
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
	WalletWIF            string
	BlockstreamAPIURL    string
	MaxTxFeeUSD          float64
	ServiceFeePercentage float64
	MinSatshiFee         int64
}

func New() *AppConfig {
	env := os.Getenv("APP_ENV")

	// this will load .env file (env from travel-exp repo)
	// this will not override env variables if they already exist
	godotenv.Load(".env." + env)

	return &AppConfig{
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
			WalletWIF:            os.Getenv("BTC_WALLET_WIF"),
			BlockstreamAPIURL:    os.Getenv("BTC_BLOCKSTREAM_API_URL"),
			MaxTxFeeUSD:          envVarAsFloat("BTC_MAX_TX_FEE_USD", 1.0),
			ServiceFeePercentage: envVarAsFloat("BTC_SERVICE_FEE_PERCENTAGE", 0.01),
			MinSatshiFee:         envVarAsInt64("BTC_MIN_SATOSHI_FEE", 3000),
		},
		Blockchain: BlockchainConfig{
			BaseRPCEndpoint:           os.Getenv("BLOCKCHAIN_BASE_RPC_ENDPOINT"),
			ICYContractAddr:           os.Getenv("BLOCKCHAIN_ICY_CONTRACT_ADDR"),
			ICYSwapContractAddr:       os.Getenv("BLOCKCHAIN_ICY_SWAP_CONTRACT_ADDR"),
			InitialICYSwapBlockNumber: envVarAtoi("BLOCKCHAIN_INITIAL_ICY_SWAP_BLOCK_NUMBER"),
			BTCTreasuryAddress:        os.Getenv("BLOCKCHAIN_BTC_TREASURY_ADDRESS"),
			InitialICYTransactionHash: os.Getenv("BLOCKCHAIN_INITIAL_ICY_TRANSACTION_HASH"),
			IcySwapSignerPrivateKey:   os.Getenv("BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY"),
		},
		IndexInterval:    os.Getenv("INDEX_INTERVAL"),
		MinIcySwapAmount: envVarAsFloat("MIN_ICY_SWAP_AMOUNT", 2000000000000000000),
	}
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
