package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/truefoundry/cruisekube/pkg/adapters/database/clients"
	"github.com/truefoundry/cruisekube/pkg/ports"
	"github.com/truefoundry/cruisekube/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DatabaseConfig holds configuration for database connections
type DatabaseConfig struct {
	Type     string `yaml:"type" json:"type"`         // "sqlite" or "postgres"
	Host     string `yaml:"host" json:"host"`         // For postgres
	Port     int    `yaml:"port" json:"port"`         // For postgres
	Database string `yaml:"database" json:"database"` // Database name or file path
	Username string `yaml:"username" json:"username"` // For postgres
	Password string `yaml:"password" json:"password"` // For postgres
	SSLMode  string `yaml:"sslmode" json:"sslmode"`   // For postgres
}

// NewDatabase creates a new storage instance based on the configuration
func NewDatabase(config DatabaseConfig) (ports.Database, error) {
	// Create the appropriate client factory
	clientFactory, err := clients.CreateClientFactory(clients.FactoryConfig{
		Type:     config.Type,
		Host:     config.Host,
		Port:     config.Port,
		Database: config.Database,
		Username: config.Username,
		Password: config.Password,
		SSLMode:  config.SSLMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client factory: %w", err)
	}

	// Create the database client
	db, err := clientFactory.CreateClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create database client: %w", err)
	}

	// Create the shared storage implementation
	return NewGormDB(db)
}

// GormDB implements the Storage interface using GORM
// This is the shared implementation that works with any GORM-supported database
type GormDB struct {
	db *gorm.DB
}

// NewGormDB creates a new GormStorage instance with the provided GORM DB client
func NewGormDB(db *gorm.DB) (*GormDB, error) {
	gormDB := &GormDB{db: db}

	// Auto-migrate the RowStats model
	if err := gormDB.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return gormDB, nil
}

func (s *GormDB) createTables() error {
	// Use GORM's AutoMigrate for both new and existing tables
	if err := s.db.AutoMigrate(&Stats{}); err != nil {
		return fmt.Errorf("failed to auto-migrate RowStats: %w", err)
	}
	if err := s.db.AutoMigrate(&OOMEvent{}); err != nil {
		return fmt.Errorf("failed to auto-migrate OOMEvent: %w", err)
	}
	return nil
}

func (s *GormDB) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	return nil
}

func (s *GormDB) UpsertStat(clusterID, workloadID string, stat types.WorkloadStat, generatedAt time.Time) error {
	statsJSON, err := json.Marshal(stat)
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	rowStat := Stats{
		ClusterID:   clusterID,
		WorkloadID:  workloadID,
		Stats:       string(statsJSON),
		GeneratedAt: generatedAt,
	}

	// Use GORM's Clauses for upsert functionality
	result := s.db.Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		Assign(Stats{
			Stats:       string(statsJSON),
			GeneratedAt: generatedAt,
		}).
		FirstOrCreate(&rowStat)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert stats: %w", result.Error)
	}

	return nil
}

func (s *GormDB) HasRecentStat(clusterID, workloadID string, withinMinutes int) (bool, error) {
	cutoffTime := time.Now().Add(-time.Duration(withinMinutes) * time.Minute)

	var count int64
	err := s.db.Model(&Stats{}).
		Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		Where("generated_at > ?", cutoffTime).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check recent stats: %w", err)
	}

	return count > 0, nil
}

func (s *GormDB) HasStatForCluster(clusterID string) (bool, error) {
	count, err := s.GetStatCountForCluster(clusterID)
	return err == nil && count > 0, nil
}

func (s *GormDB) HasStatForWorkload(clusterID, workloadID string) (bool, error) {
	count, err := s.GetStatCountForWorkload(clusterID, workloadID)
	return err == nil && count > 0, nil
}

func (s *GormDB) GetStatsForCluster(clusterID string) ([]types.WorkloadStat, error) {
	var rowStats []Stats
	err := s.db.Where(&Stats{ClusterID: clusterID}).
		Order("updated_at DESC").
		Find(&rowStats).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query cluster stats: %w", err)
	}

	var stats []types.WorkloadStat
	for _, row := range rowStats {
		var stat types.WorkloadStat
		if err := json.Unmarshal([]byte(row.Stats), &stat); err != nil {
			return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
		}

		stat.UpdatedAt = row.UpdatedAt
		stats = append(stats, stat)
	}

	return stats, nil
}

func (s *GormDB) GetStatForWorkload(clusterID, workloadID string) (*types.WorkloadStat, error) {
	var rowStat Stats
	err := s.db.Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		First(&rowStat).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workload stat not found for cluster %s, workload %s", clusterID, workloadID)
		}
		return nil, fmt.Errorf("failed to query workload stat: %w", err)
	}

	var stat types.WorkloadStat
	if err := json.Unmarshal([]byte(rowStat.Stats), &stat); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	stat.UpdatedAt = rowStat.UpdatedAt
	return &stat, nil
}

func (s *GormDB) GetStatCountForCluster(clusterID string) (int, error) {
	var count int64
	err := s.db.Model(&Stats{}).
		Where(&Stats{ClusterID: clusterID}).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count stats: %w", err)
	}

	return int(count), nil
}

func (s *GormDB) GetStatCountForWorkload(clusterID, workloadID string) (int, error) {
	var count int64
	err := s.db.Model(&Stats{}).
		Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count stats: %w", err)
	}

	return int(count), nil
}

func (s *GormDB) GetStatOverridesForWorkload(clusterID, workloadID string) (*types.Overrides, error) {
	var rowStat Stats
	err := s.db.Select("overrides").
		Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		First(&rowStat).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workload overrides not found for cluster %s, workload %s", clusterID, workloadID)
		}
		return nil, fmt.Errorf("failed to query workload overrides: %w", err)
	}

	var overrides types.Overrides
	if err := json.Unmarshal([]byte(rowStat.Overrides), &overrides); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overrides: %w", err)
	}

	return &overrides, nil
}

func (s *GormDB) DeleteStatsForCluster(clusterID string) error {
	err := s.db.Where(&Stats{ClusterID: clusterID}).Delete(&Stats{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete cluster stats: %w", err)
	}

	return nil
}

func (s *GormDB) DeleteStatForWorkload(clusterID, workloadID string) error {
	result := s.db.Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).Delete(&Stats{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete workload stat: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workload stat not found")
	}

	return nil
}

func (s *GormDB) UpdateStatOverridesForWorkload(clusterID, workloadID string, overrides *types.Overrides) error {
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("failed to marshal overrides: %w", err)
	}

	result := s.db.Model(&Stats{}).
		Where(&Stats{ClusterID: clusterID, WorkloadID: workloadID}).
		Update("overrides", string(overridesJSON))

	if result.Error != nil {
		return fmt.Errorf("failed to update workload overrides: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workload not found: cluster %s, workload %s", clusterID, workloadID)
	}

	return nil
}

func (s *GormDB) InsertOOMEvent(event *types.OOMEvent) error {
	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	dbEvent := OOMEvent{
		ClusterID:          event.ClusterID,
		ContainerID:        event.ContainerID,
		Metadata:           string(metadataJSON),
		Timestamp:          event.Timestamp,
		MemoryLimit:        event.MemoryLimit,
		MemoryRequest:      event.MemoryRequest,
		LastObservedMemory: event.LastObservedMemory,
	}

	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "container_id"}, {Name: "timestamp"}},
		DoNothing: true,
	}).Create(&dbEvent)

	if result.Error != nil {
		return fmt.Errorf("failed to insert OOM event: %w", result.Error)
	}

	return nil
}

func (s *GormDB) GetOOMEventsByWorkload(clusterID, workloadID string, since time.Time) ([]types.OOMEvent, error) {
	var dbEvents []OOMEvent
	likePattern := workloadID + ":%"
	err := s.db.Where("cluster_id = ? AND container_id LIKE ? AND timestamp >= ?", clusterID, likePattern, since).
		Order("timestamp DESC").
		Find(&dbEvents).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query OOM events: %w", err)
	}

	events := make([]types.OOMEvent, 0, len(dbEvents))
	for _, dbEvent := range dbEvents {
		var metadata types.OOMEventMetadata
		if err := json.Unmarshal([]byte(dbEvent.Metadata), &metadata); err != nil {
			// If unmarshal fails, use empty metadata
			metadata = types.OOMEventMetadata{}
		}

		events = append(events, types.OOMEvent{
			ID:                 dbEvent.ID,
			ClusterID:          dbEvent.ClusterID,
			ContainerID:        dbEvent.ContainerID,
			Metadata:           metadata,
			Timestamp:          dbEvent.Timestamp,
			MemoryLimit:        dbEvent.MemoryLimit,
			MemoryRequest:      dbEvent.MemoryRequest,
			LastObservedMemory: dbEvent.LastObservedMemory,
			CreatedAt:          dbEvent.CreatedAt,
			UpdatedAt:          dbEvent.UpdatedAt,
		})
	}

	return events, nil
}
