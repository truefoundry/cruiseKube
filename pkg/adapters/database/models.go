package database

import (
	"time"
)

type RowStats struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID   string    `json:"cluster_id" gorm:"column:clusterId;not null;index;uniqueIndex:idx_cluster_workload"`
	WorkloadID  string    `json:"workload_id" gorm:"column:workloadId;not null;index;uniqueIndex:idx_cluster_workload"`
	Stats       string    `json:"stats" gorm:"not null"`
	GeneratedAt time.Time `json:"generated_at" gorm:"column:generatedAt;not null;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updatedAt;autoUpdateTime;index"`
	Overrides   string    `json:"overrides" gorm:"default:'{}'"`
}

func (RowStats) TableName() string {
	return "stats"
}
