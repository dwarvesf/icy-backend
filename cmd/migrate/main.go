package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"

	pgstore "github.com/dwarvesf/icy-backend/internal/store/postgres"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func runMigrations(db *gorm.DB, logger *logger.Logger) error {
	// Open database connection
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Create migrate instance
	migrationPath := fmt.Sprintf("file://%s", filepath.Join("migrations", "schema"))
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		migrationPath,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	logger.Info("Migrations completed successfully")
	return nil
}

func main() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)

	db := pgstore.New(appConfig, logger)

	if err := runMigrations(db, logger); err != nil {
		logger.Error("[main][runMigrations] failed to run migrations", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
}
