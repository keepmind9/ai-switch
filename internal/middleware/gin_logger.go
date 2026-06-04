package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// GinLogger returns a gin middleware that logs request details via slog,
// producing output similar to gin.Logger() but routed through the structured
// logger so entries appear in both console and the app log file.
//
// Format: [GIN] 2026/06/04 - 08:46:01 | 200 | 5.593s | 127.0.0.1 | POST "/v1/messages"
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
