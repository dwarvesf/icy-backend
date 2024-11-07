package store

import (
	"fmt"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// NewPostgresStore postgres init by gorm
func NewPostgresStore(appConfig *config.AppConfig, logger *logger.Logger) DBRepo {
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
		logger.Fatal("failed to open database connection", map[string]string{
			"error": err.Error(),
		})
	}

	logger.Info("database connected")
	return &repo{Database: db}
}
