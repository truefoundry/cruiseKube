package config

import (
	"github.com/gin-gonic/gin"
)

func GetConfigFromGinContext(c *gin.Context) *Config {
	return c.MustGet("appConfig").(*Config) //nolint:forcetypeassert
}
