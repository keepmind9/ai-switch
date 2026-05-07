package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestIPWhitelist_LocalhostSkipsCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run("host="+host, func(t *testing.T) {
			r := gin.New()
			r.Use(IPWhitelist(host, nil))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestIPWhitelist_NonLocalhostAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		allowedIPs []string
		remoteAddr string
		wantStatus int
	}{
		{
			"matching CIDR",
			[]string{"192.168.1.0/24"},
			"192.168.1.5:12345",
			http.StatusOK,
		},
		{
			"matching bare IP normalized to /32",
			[]string{"10.0.0.5"},
			"10.0.0.5:12345",
			http.StatusOK,
		},
		{
			"non-matching IP",
			[]string{"192.168.1.0/24"},
			"10.0.0.1:12345",
			http.StatusForbidden,
		},
		{
			"multiple CIDRs match second",
			[]string{"10.0.0.0/8", "172.16.0.0/12"},
			"172.16.5.5:12345",
			http.StatusOK,
		},
		{
			"IPv6 match",
			[]string{"::1/128"},
			"[::1]:12345",
			http.StatusOK,
		},
		{
			"IPv6 bare IP",
			[]string{"2001:db8::1"},
			"[2001:db8::1]:12345",
			http.StatusOK,
		},
		{
			"empty whitelist rejects all",
			[]string{},
			"192.168.1.1:12345",
			http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(IPWhitelist("0.0.0.0", tt.allowedIPs))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestIPWhitelist_InvalidCIDRPanics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	assert.Panics(t, func() {
		IPWhitelist("0.0.0.0", []string{"not-a-cidr"})
	})
}

func TestNormalizeCIDR(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.0/24", "192.168.1.0/24"},
		{"10.0.0.5", "10.0.0.5/32"},
		{"::1", "::1/128"},
		{"2001:db8::/32", "2001:db8::/32"},
		{"garbage", "garbage"}, // left as-is, will fail ParseCIDR later
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeCIDR(tt.input))
		})
	}
}
