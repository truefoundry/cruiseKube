package database

import (
	"time"
)

type Stats struct {
	ID          uint      `gorm:"column:id;primaryKey;autoIncrement"`
	ClusterID   string    `gorm:"column:cluster_id;index;"`
	WorkloadID  string    `gorm:"column:workload_id;index;"`
	Stats       string    `gorm:"column:stats"`
	GeneratedAt time.Time `gorm:"column:generated_at;index"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
	Overrides   string    `gorm:"column:overrides;default:'{}'"`
}

func (Stats) TableName() string {
	return "stats"
}

type OOMEvent struct {
	ID                 uint      `gorm:"column:id;primaryKey;autoIncrement"`
	ClusterID          string    `gorm:"column:cluster_id;index;uniqueIndex:idx_oom_unique"`
	ContainerID        string    `gorm:"column:container_id;index;uniqueIndex:idx_oom_unique"`
	Timestamp          time.Time `gorm:"column:timestamp;index;uniqueIndex:idx_oom_unique"`
	MemoryLimit        int64     `gorm:"column:memory_limit;"`
	MemoryRequest      int64     `gorm:"column:memory_request;"`
	LastObservedMemory int64     `gorm:"column:last_observed_memory;"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (OOMEvent) TableName() string {
	return "oom_events"
}
