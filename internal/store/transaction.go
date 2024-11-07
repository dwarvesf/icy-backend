package store

import (
	"gorm.io/gorm"
)

// NewTransaction for database connection
func DoInTx(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	err := fn(tx)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
