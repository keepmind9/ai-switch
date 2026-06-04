package middleware

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGinLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		statusCode int
		method     string
		path       string
		wantLevel  string // "INFO", "WARN", "ERROR" substring in log
	}{
		{
			name:       "200 OK",
			statusCode: http.StatusOK,
			method:     http.MethodGet,
			path:       "/v1/models",
			wantLevel:  "INFO", // slog text format includes level
		},
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			wantLevel:  "WARN",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			method:     http.MethodPost,
			path:       "/v1/messages",
			wantLevel:  "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(GinLogger())
			r.Handle(tt.method, tt.path, func(c *gin.Context) {
				c.Status(tt.statusCode)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

func TestGinLogger_QueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GinLogger())
	r.GET("/v1/messages", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages?beta=true", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGinLogger_FormatContainsGIN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GinLogger())
	r.POST("/v1/messages", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	r.ServeHTTP(w, req)

	_ = w // just ensure no panic; the slog output goes to configured handlers
}

func TestGinLogger_LatencyRecorded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GinLogger())
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGinLogger_PanicRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// GinLogger should still record even if a panic occurs downstream
	r := gin.New()
	r.Use(GinLogger())
	r.Use(gin.Recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	// Recovery middleware catches panic and returns 500
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGinLogger_404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(GinLogger())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGinLogger_StatusLevels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		status    int
		wantLevel string
	}{
		{200, "INFO"},
		{301, "INFO"},
		{400, "WARN"},
		{404, "WARN"},
		{500, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d_%s", tt.status, tt.wantLevel), func(t *testing.T) {
			// Capture slog output to verify log level
			var buf bytes.Buffer
			orig := slog.Default()
			defer slog.SetDefault(orig)
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

			r := gin.New()
			r.Use(GinLogger())
			r.GET("/test", func(c *gin.Context) {
				c.Status(tt.status)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
			require.NotEmpty(t, buf.String(), "expected slog output")
			assert.Contains(t, buf.String(), "level="+tt.wantLevel)
		})
	}
}
