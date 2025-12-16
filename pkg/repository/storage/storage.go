package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/truefoundry/cruisekube/pkg/ports"
	"github.com/truefoundry/cruisekube/pkg/types"
)

var Stg *Storage

type Storage struct {
	DB ports.Database
}

func NewStorageRepo(db ports.Database) (*Storage, error) {
	return &Storage{DB: db}, nil
}

func (s *Storage) WriteClusterStats(clusterID string, statsResponse types.StatsResponse, generatedAt time.Time) error {
	for _, stat := range statsResponse.Stats {
		workloadID := strings.ReplaceAll(stat.WorkloadIdentifier, "/", ":")
		if err := s.DB.UpsertStat(clusterID, workloadID, stat, generatedAt); err != nil {
			return fmt.Errorf("failed to write workload stat %s: %w", workloadID, err)
		}
	}

	return nil
}

func (s *Storage) ReadClusterStats(clusterID string, target *types.StatsResponse) error {
	stats, err := s.DB.GetStatsForCluster(clusterID)
	if err != nil {
		return fmt.Errorf("failed to read cluster stats: %w", err)
	}

	target.Stats = stats
	return nil
}

func (s *Storage) ClusterStatsExists(clusterID string) (bool, error) {
	return s.DB.HasStatForCluster(clusterID)
}

func (s *Storage) HasRecentStats(clusterID, workloadID string, withinMinutes int) (bool, error) {
	return s.DB.HasRecentStat(clusterID, workloadID, withinMinutes)
}

func (s *Storage) UpdateWorkloadOverrides(clusterID, workloadID string, overrides *types.Overrides) error {
	exists, err := s.DB.HasStatForWorkload(clusterID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to get stats record: %w", err)
	}
	if !exists {
		return fmt.Errorf("stats record not found")
	}
	return s.DB.UpdateStatOverridesForWorkload(clusterID, workloadID, overrides)
}

func (s *Storage) GetWorkloadOverrides(clusterID, workloadID string) (*types.Overrides, error) {
	return s.DB.GetStatOverridesForWorkload(clusterID, workloadID)
}

func (s *Storage) GetAllStatsForCluster(clusterID string) ([]types.WorkloadStat, error) {
	return s.DB.GetStatsForCluster(clusterID)
}
