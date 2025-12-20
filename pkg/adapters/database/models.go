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
