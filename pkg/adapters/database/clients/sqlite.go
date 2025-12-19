package clients

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQLiteClientFactory creates SQLite database clients
type SQLiteClientFactory struct {
	dbPath string
}

// NewSQLiteClientFactory creates a new SQLite client factory
func NewSQLiteClientFactory(dbPath string) *SQLiteClientFactory {
	return &SQLiteClientFactory{dbPath: dbPath}
}

func (f *SQLiteClientFactory) CreateClient() (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(f.dbPath), 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(f.dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure connection pool for SQLite
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite works best with single connection

	return db, nil
}
