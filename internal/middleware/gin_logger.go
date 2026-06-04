package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// GinLogger returns a gin middleware that logs request details via slog,
// so entries appear in both console and the app log file.
// Logs are structured with status, latency, client_ip, method, path fields.
// Log level: INFO for 1xx-3xx, WARN for 4xx, ERROR for 5xx.
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if raw := c.Request.URL.RawQuery; raw != "" {
			path = path + "?" + raw
		}

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		msg := fmt.Sprintf("[GIN] %d %s %s", statusCode, method, path)

		attrs := []any{
			"status", statusCode,
			"latency", latency.String(),
			"client_ip", clientIP,
			"method", method,
			"path", path,
		}

		switch {
		case statusCode >= 500:
			slog.Error(msg, attrs...)
		case statusCode >= 400:
			slog.Warn(msg, attrs...)
		default:
			slog.Info(msg, attrs...)
		}
	}
}
