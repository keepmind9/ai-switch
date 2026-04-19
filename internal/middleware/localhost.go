package middleware

import (
	"net"

	"github.com/gin-gonic/gin"
)

// LocalhostOnly returns middleware that restricts access to loopback addresses.
func LocalhostOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil || !ip.IsLoopback() {
			c.AbortWithStatusJSON(403, gin.H{"error": "admin access restricted to localhost"})
			return
		}
		c.Next()
	}
}
