package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStorage struct {
	conn *sql.DB
}

func NewSQLiteAdapter(dbPath string) (*SQLiteStorage, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(1)

	db := &SQLiteStorage{conn: conn}
	if err := db.createTables(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}

func (db *SQLiteStorage) Close() error {
	if err := db.conn.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	return nil
}

func (db *SQLiteStorage) createTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		clusterId TEXT NOT NULL,
		workloadId TEXT NOT NULL,
		stats TEXT NOT NULL,
		overrides TEXT DEFAULT '{}',
		generatedAt DATETIME NOT NULL,
		createdAt DATETIME DEFAULT CURRENT_TIMESTAMP,
		updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(clusterId, workloadId)
	);

	CREATE INDEX IF NOT EXISTS idx_cluster_workload ON stats(clusterId, workloadId);
	CREATE INDEX IF NOT EXISTS idx_cluster_updated ON stats(clusterId, updatedAt);
	CREATE INDEX IF NOT EXISTS idx_cluster_generated ON stats(clusterId, generatedAt);

	CREATE TRIGGER IF NOT EXISTS update_stats_timestamp 
		AFTER UPDATE ON stats
		BEGIN
			UPDATE stats SET updatedAt = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;
	`

	_, err := db.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	migrationQuery := `
	ALTER TABLE stats ADD COLUMN overrides TEXT DEFAULT '{}';
	`

	_, _ = db.conn.Exec(migrationQuery)

	return nil
}
