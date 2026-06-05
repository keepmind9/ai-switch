package admincli

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/keepmind9/ai-switch/internal/config"
)

// ResolveAdminURL returns the admin API base URL (e.g. "http://127.0.0.1:12345/api").
// Priority: urlFlag > config file > defaults (127.0.0.1:12345).
func ResolveAdminURL(configPath, urlFlag string) (string, error) {
	if urlFlag != "" {
		parsed, err := url.Parse(urlFlag)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return "", fmt.Errorf("invalid --url value %q: must be a valid http/https URL", urlFlag)
		}
		return strings.TrimRight(urlFlag, "/") + "/api", nil
	}

	host := config.DefaultHost
	port := config.DefaultPort

	resolvedPath, _ := config.DefaultConfigPath(configPath)
	if resolvedPath != "" {
		if _, err := os.Stat(resolvedPath); err == nil {
			cfg, err := config.Load(resolvedPath)
			if err != nil {
				return "", fmt.Errorf("failed to load config from %s: %w", resolvedPath, err)
			}
			host = cfg.Server.Host
			port = cfg.Server.Port
		}
	}

	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/api", host, port), nil
}
