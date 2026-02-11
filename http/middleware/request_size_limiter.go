package middleware

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// 默认限制为 1MB
	DefaultMaxSize = 1 * 1024 * 1024
)

// RequestSizeLimiter 限制请求体大小的中间件
func RequestSizeLimiter(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxSize <= 0 {
			maxSize = DefaultMaxSize
		}

		if c.Request.ContentLength > maxSize {
			errorMsg := fmt.Sprintf("请求体过大，最大允许 %s", formatBytes(maxSize))
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": errorMsg,
				"code":  http.StatusRequestEntityTooLarge,
			})
			c.Abort()
			return
		}

		// 包装 Request.Body 以限制读取大小
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

		defer func() {
			// 确保body被完全读取，防止连接被挂起
			_, _ = io.Copy(io.Discard, c.Request.Body)
		}()

		c.Next()
	}
}

// RequestSizeLimiterDefault 使用默认大小限制(1MB)的中间件
func RequestSizeLimiterDefault() gin.HandlerFunc {
	return RequestSizeLimiter(DefaultMaxSize)
}

// formatBytes 将字节数格式化为人类可读的字符串
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
