package middleware

import (
	"gameserver/core/conf"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Logger 日志中间件
func Logger() gin.HandlerFunc {
	logrus.SetLevel(GetLogLevel())
	return gin.LoggerWithWriter(logrus.StandardLogger().Writer())
}

func GetLogLevel() logrus.Level {
	switch conf.LogLevel {
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	default:
		return logrus.FatalLevel
	}
}
