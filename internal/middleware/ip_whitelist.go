package middleware

import (
	"fmt"
	"net"

	"github.com/gin-gonic/gin"
)

// IPWhitelist returns middleware that rejects requests from IPs not in the
// allowed list. If host is 127.0.0.1 or localhost, all checks are skipped.
func IPWhitelist(host string, allowedIPs []string) gin.HandlerFunc {
	if host == "127.0.0.1" || host == "localhost" {
		return func(c *gin.Context) { c.Next() }
	}

	nets := make([]*net.IPNet, 0, len(allowedIPs))
	for _, s := range allowedIPs {
		_, ipNet, err := net.ParseCIDR(normalizeCIDR(s))
		if err != nil {
			panic(fmt.Sprintf("middleware/IPWhitelist: invalid CIDR %q: %v", s, err))
		}
		nets = append(nets, ipNet)
	}

	return func(c *gin.Context) {
		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil {
			c.AbortWithStatusJSON(403, gin.H{"error": "access denied: invalid client IP"})
			return
		}
		for _, n := range nets {
			if n.Contains(clientIP) {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"error": "access denied: IP not in whitelist"})
	}
}

// normalizeCIDR converts a bare IP address to CIDR notation.
func normalizeCIDR(s string) string {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return s
	}
	if ip := net.ParseIP(s); ip != nil {
		if ip.To4() != nil {
			return s + "/32"
		}
		return s + "/128"
	}
	return s
}
