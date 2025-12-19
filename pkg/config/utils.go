package config

import (
	"github.com/gin-gonic/gin"
)

func GetConfigFromGinContext(c *gin.Context) *Config {
	appConfig, ok := c.MustGet("appConfig").(*Config)
	if !ok {
		panic("invalid app config type")
	}
	return appConfig
}
