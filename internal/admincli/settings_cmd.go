package admincli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func NewSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage server settings",
	}

	cmd.AddCommand(
		newSettingsGetCmd(),
		newSettingsUpdateCmd(),
	)
	return cmd
}

// SettingsItem mirrors the API response for GET /admin/settings.
type SettingsItem struct {
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	AllowedIPs       []string `json:"allowed_ips"`
	LogRetentionDays int      `json:"log_retention_days"`
	ProxyURL         string   `json:"proxy_url"`
}

func newSettingsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show current server settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/settings")
			if err != nil {
				return err
			}

			var s SettingsItem
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(s)
			}

			headers := []string{"HOST", "PORT", "PROXY URL", "LOG RETENTION", "ALLOWED IPS"}
			rows := [][]string{{
				s.Host,
				fmt.Sprintf("%d", s.Port),
				dashIfEmpty(s.ProxyURL),
				fmt.Sprintf("%d days", s.LogRetentionDays),
				joinSlice(s.AllowedIPs),
			}}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newSettingsUpdateCmd() *cobra.Command {
	var host string
	var port int
	var logRetention int
	var proxyURL string
	var allowedIPs []string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update server settings",
		Example: "  ais admin settings update --port 8080\n" +
			"  ais admin settings update --proxy-url socks5://127.0.0.1:1080",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := make(map[string]any)
			if cmd.Flags().Changed("host") {
				req["host"] = host
			}
			if cmd.Flags().Changed("port") {
				if port < 1 || port > 65535 {
					return fmt.Errorf("port must be between 1 and 65535")
				}
				req["port"] = port
			}
			if cmd.Flags().Changed("log-retention-days") {
				if logRetention < 1 {
					return fmt.Errorf("log-retention-days must be at least 1")
				}
				req["log_retention_days"] = logRetention
			}
			if cmd.Flags().Changed("proxy-url") {
				req["proxy_url"] = proxyURL
			}
			if cmd.Flags().Changed("allowed-ips") {
				req["allowed_ips"] = allowedIPs
			}

			if len(req) == 0 {
				return fmt.Errorf("specify at least one setting to update")
			}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/settings", req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Server bind host")
	cmd.Flags().IntVar(&port, "port", 0, "Server bind port (1-65535)")
	cmd.Flags().IntVar(&logRetention, "log-retention-days", 0, "Log retention days (minimum 1)")
	cmd.Flags().StringVar(&proxyURL, "proxy-url", "", "Global proxy URL (http/https/socks5)")
	cmd.Flags().StringSliceVar(&allowedIPs, "allowed-ips", nil, "Allowed IP/CIDR list (replaces all)")
	bindOutputFlag(cmd)
	return cmd
}

// dashIfEmpty returns "-" for empty strings.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
