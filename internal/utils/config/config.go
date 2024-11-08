package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
)

type AppConfig struct {
	Environment environments.Environment
	ApiServer   ApiServerConfig
	Postgres    DBConnection
	Bitcoin     BitcoinConfig
}

type ApiServerConfig struct {
	AllowedOrigins string
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
}

func New() *AppConfig {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	// this will load .env file (env from travel-exp repo)
	// this will not override env variables if they already exist
	godotenv.Load(".env." + env)

	return &AppConfig{
		ApiServer: ApiServerConfig{
			AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
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
			WalletWIF:         os.Getenv("BTC_WALLET_WIF"),
			BlockstreamAPIURL: os.Getenv("BTC_BLOCKSTREAM_API_URL"),
		},
	}
}

func envVarAtoi(envName string) int {
	valueStr := os.Getenv(envName)
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(err)
	}

	return value
}

func envVarAsBool(envName string) bool {
	valueStr := os.Getenv(envName)
	return valueStr == "true"
}
