package admincli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func NewSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System operations",
	}

	cmd.AddCommand(
		newStatusCmd(),
		newPresetListCmd(),
		newRestartCmd(),
		newStopCmd(),
	)
	return cmd
}

// StatusItem mirrors the API response for GET /admin/status.
type StatusItem struct {
	Server                any    `json:"server"`
	DefaultRoute          string `json:"default_route"`
	DefaultAnthropicRoute string `json:"default_anthropic_route"`
	DefaultResponsesRoute string `json:"default_responses_route"`
	DefaultChatRoute      string `json:"default_chat_route"`
	ProviderCount         int    `json:"provider_count"`
	RouteCount            int    `json:"route_count"`
}

// PresetItem mirrors the API response for GET /admin/presets.
type PresetItem struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Format    string `json:"format"`
	Category  string `json:"category"`
	IsPartner bool   `json:"is_partner"`
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/status")
			if err != nil {
				return err
			}

			var s StatusItem
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(s)
			}

			headers := []string{"PROVIDERS", "ROUTES", "DEFAULT", "ANTHROPIC", "RESPONSES", "CHAT"}
			rows := [][]string{{
				fmt.Sprintf("%d", s.ProviderCount),
				fmt.Sprintf("%d", s.RouteCount),
				dashIfEmpty(s.DefaultRoute),
				dashIfEmpty(s.DefaultAnthropicRoute),
				dashIfEmpty(s.DefaultResponsesRoute),
				dashIfEmpty(s.DefaultChatRoute),
			}}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newPresetListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset-list",
		Short: "List provider presets",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/presets")
			if err != nil {
				return err
			}

			var presets []PresetItem
			if err := json.Unmarshal(data, &presets); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(presets)
			}

			headers := []string{"KEY", "NAME", "FORMAT", "CATEGORY"}
			rows := make([][]string, len(presets))
			for i, p := range presets {
				rows[i] = []string{p.Key, p.Name, p.Format, p.Category}
			}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart server",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			_, err := client.Post(cmd.Context(), "/restart", nil)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintMessage("server restarting...")
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop server",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			_, err := client.Post(cmd.Context(), "/stop", nil)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintMessage("server stopped")
		},
	}
	bindOutputFlag(cmd)
	return cmd
}
