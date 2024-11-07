package pgstore

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type PostgresStore struct {
	// db *gorm.DB
}

func New(appConfig *config.AppConfig, logger *logger.Logger) *PostgresStore {
	_, err := connectPostgres(appConfig)
	if err != nil {
		logger.Fatal("failed to connect to postgres", map[string]string{
			"error": err.Error(),
		})
	}

	return &PostgresStore{
		// db: conn,
	}
}

func connectPostgres(appConfig *config.AppConfig) (*gorm.DB, error) {
	ds := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		appConfig.Postgres.Host,
		appConfig.Postgres.User,
		appConfig.Postgres.Pass,
		appConfig.Postgres.Name,
		appConfig.Postgres.Port,
		appConfig.Postgres.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(ds),
		&gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
	if err != nil {
		return nil, err
	}

	return db, nil
}
