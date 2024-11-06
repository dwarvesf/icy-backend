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
}

type ApiServerConfig struct {
	AllowedOrigins string
}

func New() *AppConfig {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	// this will load .env file (env from travel-exp repo)
	// this will not override env variables if they already exist
	godotenv.Load(".env." + env)

	return &AppConfig{}
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
