package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/handler"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/require"
)

// protocolTestEnv holds the in-process ai-switch gateway + mock upstream for mode A tests.
type protocolTestEnv struct {
	gateway  *httptest.Server
	upstream *httptest.Server
	config   *config.Config
}

// setupProtocolTest creates an in-process ai-switch pointing to a mock upstream.
func setupProtocolTest(t *testing.T, upstreamFormat string) *protocolTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mock := newMockUpstream(upstreamFormat)
	ts := httptest.NewServer(mock)
	t.Cleanup(ts.Close)

	cfg := &config.Config{
		DefaultRoute: "test-route",
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Name:    "Mock",
				BaseURL: ts.URL,
				APIKey:  "mock-key",
				Format:  upstreamFormat,
			},
		},
		Routes: map[string]config.RouteRule{
			"test-key": {
				Provider:     "mock",
				DefaultModel: "test-model",
			},
			"test-route": {
				Provider:     "mock",
				DefaultModel: "test-model",
			},
		},
	}

	provider := config.NewProvider(cfg, "")
	r := router.NewConfigRouter(provider)
	h := handler.NewHandler(provider, nil, r, nil)
	engine := gin.New()
	h.RegisterRoutes(engine)

	gw := httptest.NewServer(engine)
	t.Cleanup(gw.Close)

	return &protocolTestEnv{
		gateway:  gw,
		upstream: ts,
		config:   cfg,
	}
}

// cliTestEnv holds the ai-switch subprocess for mode B (real CLI) tests.
type cliTestEnv struct {
	serverCmd  *exec.Cmd
	serverURL  string
	configPath string
	tmpDir     string
}

// setupCLITest builds and starts ai-switch as a subprocess using config.example.yaml.
func setupCLITest(t *testing.T) *cliTestEnv {
	t.Helper()

	// Find config.example.yaml
	configPath := filepath.Join("testdata", "config.example.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("testdata/config.example.yaml not found — fill in the template first")
	}

	// Check if config has been filled in
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	if string(data) != "" {
		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err == nil {
			if providers, ok := cfg["providers"].(map[string]any); ok {
				for _, p := range providers {
					if prov, ok := p.(map[string]any); ok {
						if url, _ := prov["base_url"].(string); url == "<FILL>" {
							t.Skip("testdata/config.example.yaml contains <FILL> placeholders — fill in real values first")
						}
					}
				}
			}
		}
	}

	tmpDir := t.TempDir()

	// Build binary
	binPath := filepath.Join(tmpDir, "ai-switch-test-server")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/server")
	buildCmd.Dir = projectRoot(t)
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Copy config to tmpdir with a random port
	tmpConfig := filepath.Join(tmpDir, "config.yaml")
	copyConfigWithPort(t, configPath, tmpConfig, 0)

	// Start subprocess
	cmd := exec.Command(binPath, "-c", tmpConfig)
	cmd.Dir = projectRoot(t)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "failed to start ai-switch subprocess")

	// Find the assigned port from the generated config
	port := readConfigPort(t, tmpConfig)
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Health check — wait up to 10s
	require.Eventually(t, func() bool {
		resp, err := http.Get(serverURL + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 200*time.Millisecond, "ai-switch did not start")

	env := &cliTestEnv{
		serverCmd:  cmd,
		serverURL:  serverURL,
		configPath: tmpConfig,
		tmpDir:     tmpDir,
	}

	t.Cleanup(env.teardown)
	return env
}

func (e *cliTestEnv) teardown() {
	if e.serverCmd != nil && e.serverCmd.Process != nil {
		e.serverCmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- e.serverCmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			e.serverCmd.Process.Kill()
		}
	}
}

// routeKeys returns all route keys from config.example.yaml for matrix testing.
func (e *cliTestEnv) routeKeys() []string {
	data, err := os.ReadFile(e.configPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Routes map[string]any `yaml:"routes"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	keys := make([]string, 0, len(cfg.Routes))
	for k := range cfg.Routes {
		keys = append(keys, k)
	}
	return keys
}

// requireCLI skips the test if the named CLI is not installed.
func requireCLI(t *testing.T, name string) {
	t.Helper()
	_, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed, skipping", name)
	}
}

// projectRoot returns the project root directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	// We're in tests/e2e/, go up two levels
	return filepath.Join(dir, "..", "..")
}

// copyConfigWithPort copies a config file, overriding the port.
func copyConfigWithPort(t *testing.T, src, dst string, port int) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0644))
}

// readConfigPort reads the port from a YAML config file.
func readConfigPort(t *testing.T, path string) int {
	t.Helper()
	// Simple approach: use config.Load
	cfg, err := config.Load(path)
	require.NoError(t, err)
	if cfg.Server.Port == 0 {
		return config.DefaultPort
	}
	return cfg.Server.Port
}

// readBody reads and returns the response body as a string.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}
