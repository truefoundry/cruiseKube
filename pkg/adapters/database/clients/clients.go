package clients

import (
	"gorm.io/gorm"
)

const (
	TYPE_SQLITE   = "sqlite"
	TYPE_POSTGRES = "postgres"
	TYPE_DEFAULT  = ""
)

// FactoryConfig holds configuration for database connections
type FactoryConfig struct {
	Type     string `yaml:"type" json:"type"`         // "sqlite" or "postgres"
	Host     string `yaml:"host" json:"host"`         // For postgres
	Port     int    `yaml:"port" json:"port"`         // For postgres
	Database string `yaml:"database" json:"database"` // Database name or file path
	Username string `yaml:"username" json:"username"` // For postgres
	Password string `yaml:"password" json:"password"` // For postgres
	SSLMode  string `yaml:"sslmode" json:"sslmode"`   // For postgres
}

// ClientFactory defines the interface for creating database clients
type ClientFactory interface {
	CreateClient() (*gorm.DB, error)
}

// createClientFactory creates the appropriate client factory based on database type
func CreateClientFactory(config FactoryConfig) (ClientFactory, error) {
	switch config.Type {
	case TYPE_SQLITE, TYPE_DEFAULT:
		return NewSQLiteClientFactory(config.Database), nil
	case TYPE_POSTGRES:
		return NewPostgreSQLClientFactory(config), nil
	default:
		return NewSQLiteClientFactory(config.Database), nil
	}
}
