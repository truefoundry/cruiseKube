package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/truefoundry/cruisekube/pkg/types"
)

func (db *SQLiteStorage) UpsertStat(clusterID, workloadID string, stat types.WorkloadStat, generatedAt time.Time) error {
	statsJSON, err := json.Marshal(stat)
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	query := `
		INSERT INTO stats (clusterId, workloadId, stats, generatedAt) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT(clusterId, workloadId) 
		DO UPDATE SET 
			stats = excluded.stats,
			generatedAt = excluded.generatedAt,
			updatedAt = CURRENT_TIMESTAMP
	`

	_, err = db.conn.Exec(query, clusterID, workloadID, string(statsJSON), generatedAt.Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("failed to insert/update stats: %w", err)
	}

	return nil
}

func (db *SQLiteStorage) HasRecentStat(clusterID, workloadID string, withinMinutes int) (bool, error) {
	query := `
		SELECT COUNT(*) FROM stats 
		WHERE clusterId = ? AND workloadId = ? 
		AND generatedAt > datetime('now', '-' || ? || ' minutes')
	`

	var count int
	err := db.conn.QueryRow(query, clusterID, workloadID, withinMinutes).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check recent stats: %w", err)
	}

	return count > 0, nil
}

func (db *SQLiteStorage) HasStatForCluster(clusterID string) (bool, error) {
	count, err := db.GetStatCountForCluster(clusterID)
	return err == nil && count > 0, nil
}

func (db *SQLiteStorage) HasStatForWorkload(clusterID, workloadID string) (bool, error) {
	count, err := db.GetStatCountForWorkload(clusterID, workloadID)
	return err == nil && count > 0, nil
}

func (db *SQLiteStorage) GetStatsForCluster(clusterID string) ([]types.WorkloadStat, error) {
	query := `SELECT stats, updatedAt FROM stats WHERE clusterId = ? ORDER BY updatedAt DESC`

	rows, err := db.conn.Query(query, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to query cluster stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []types.WorkloadStat
	for rows.Next() {
		var statsJSON string
		var updatedAt time.Time
		if err := rows.Scan(&statsJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var stat types.WorkloadStat
		if err := json.Unmarshal([]byte(statsJSON), &stat); err != nil {
			return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
		}

		stat.UpdatedAt = updatedAt
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return stats, nil
}

func (db *SQLiteStorage) GetStatForWorkload(clusterID, workloadID string) (*types.WorkloadStat, error) {
	query := `SELECT stats, updatedAt FROM stats WHERE clusterId = ? AND workloadId = ?`

	var statsJSON string
	var updatedAt time.Time
	err := db.conn.QueryRow(query, clusterID, workloadID).Scan(&statsJSON, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workload stat not found for cluster %s, workload %s", clusterID, workloadID)
		}
		return nil, fmt.Errorf("failed to query workload stat: %w", err)
	}

	var stat types.WorkloadStat
	if err := json.Unmarshal([]byte(statsJSON), &stat); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	stat.UpdatedAt = updatedAt
	return &stat, nil
}

func (db *SQLiteStorage) GetStatCountForCluster(clusterID string) (int, error) {
	query := `SELECT COUNT(*) FROM stats WHERE clusterId = ?`

	var count int
	err := db.conn.QueryRow(query, clusterID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count stats: %w", err)
	}

	return count, nil
}

func (db *SQLiteStorage) GetStatCountForWorkload(clusterID, workloadID string) (int, error) {
	query := `SELECT COUNT(*) FROM stats WHERE clusterId = ? AND workloadId = ?`

	var count int
	err := db.conn.QueryRow(query, clusterID, workloadID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count stats: %w", err)
	}

	return count, nil
}

func (db *SQLiteStorage) GetStatOverridesForWorkload(clusterID, workloadID string) (*types.Overrides, error) {
	query := `SELECT overrides FROM stats WHERE clusterId = ? AND workloadId = ?`

	var overridesJSON string
	err := db.conn.QueryRow(query, clusterID, workloadID).Scan(&overridesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workload overrides not found for cluster %s, workload %s", clusterID, workloadID)
		}
		return nil, fmt.Errorf("failed to query workload overrides: %w", err)
	}

	var overrides types.Overrides
	if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overrides: %w", err)
	}

	return &overrides, nil
}

func (db *SQLiteStorage) DeleteStatsForCluster(clusterID string) error {
	query := `DELETE FROM stats WHERE clusterId = ?`

	_, err := db.conn.Exec(query, clusterID)
	if err != nil {
		return fmt.Errorf("failed to delete cluster stats: %w", err)
	}

	return nil
}

func (db *SQLiteStorage) DeleteStatForWorkload(clusterID, workloadID string) error {
	query := `DELETE FROM stats WHERE clusterId = ? AND workloadId = ?`

	result, err := db.conn.Exec(query, clusterID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to delete workload stat: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workload stat not found")
	}

	return nil
}

func (db *SQLiteStorage) UpdateStatOverridesForWorkload(clusterID, workloadID string, overrides *types.Overrides) error {
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("failed to marshal overrides: %w", err)
	}

	query := `
		UPDATE stats 
		SET overrides = ?, updatedAt = CURRENT_TIMESTAMP 
		WHERE clusterId = ? AND workloadId = ?
	`

	result, err := db.conn.Exec(query, string(overridesJSON), clusterID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to update workload overrides: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workload not found: cluster %s, workload %s", clusterID, workloadID)
	}

	return nil
}
