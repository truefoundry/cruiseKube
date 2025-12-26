package ports

import (
	"time"

	"github.com/truefoundry/cruisekube/pkg/types"
)

type Database interface {
	Close() error
	// Upsert
	UpsertStat(clusterID, workloadID string, stat types.WorkloadStat, generatedAt time.Time) error

	// Has
	HasRecentStat(clusterID, workloadID string, withinMinutes int) (bool, error)
	HasStatForCluster(clusterID string) (bool, error)
	HasStatForWorkload(clusterID, workloadID string) (bool, error)

	// Get
	GetStatsForCluster(clusterID string) ([]types.WorkloadStat, error)
	GetStatForWorkload(clusterID, workloadID string) (*types.WorkloadStat, error)
	GetStatCountForCluster(clusterID string) (int, error)
	GetStatCountForWorkload(clusterID, workloadID string) (int, error)
	GetStatOverridesForWorkload(clusterID, workloadID string) (*types.Overrides, error)

	// Delete
	DeleteStatsForCluster(clusterID string) error
	DeleteStatForWorkload(clusterID, workloadID string) error

	// Update
	UpdateStatOverridesForWorkload(clusterID, workloadID string, overrides *types.Overrides) error

	// OOM Events
	InsertOOMEvent(event *types.OOMEvent) error
	GetOOMEventsByWorkload(clusterID, workloadID string, since time.Time) ([]types.OOMEvent, error)
}
