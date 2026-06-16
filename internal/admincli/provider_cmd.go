package admincli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// parseHeaderFlags parses repeatable "Key: Value" flags (curl -H convention)
// into a map. A header without a colon becomes an empty value. Returns nil when
// the slice is empty so callers can omit the field from partial-update payloads.
func parseHeaderFlags(headers []string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		k, v, _ := strings.Cut(h, ":")
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m
}

// --- Provider types mirroring API JSON shapes ---

// ProviderItem is a single provider in list output.
type ProviderItem struct {
	Key           string            `json:"key"`
	Name          string            `json:"name"`
	BaseURL       string            `json:"base_url"`
	Format        string            `json:"format"`
	Models        []string          `json:"models"`
	EnableProxy   bool              `json:"enable_proxy"`
	APIKey        string            `json:"api_key"`
	Path          string            `json:"path"`
	LogoURL       string            `json:"logo_url"`
	ThinkTag      string            `json:"think_tag"`
	CustomHeaders map[string]string `json:"custom_headers"`
}

// CreateProviderReq is the request body for POST /admin/providers.
type CreateProviderReq struct {
	Key           string            `json:"key"`
	Name          string            `json:"name"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key"`
	Format        string            `json:"format"`
	Path          string            `json:"path,omitempty"`
	LogoURL       string            `json:"logo_url,omitempty"`
	ThinkTag      string            `json:"think_tag,omitempty"`
	FallbackKeys  []string          `json:"fallback_keys,omitempty"`
	Models        []string          `json:"models,omitempty"`
	DefaultModel  string            `json:"default_model,omitempty"`
	EnableProxy   bool              `json:"enable_proxy"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
}

func NewProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage providers",
	}

	cmd.AddCommand(
		newProviderListCmd(),
		newProviderAddCmd(),
		newProviderUpdateCmd(),
		newProviderRemoveCmd(),
	)
	return cmd
}

func newProviderListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/providers")
			if err != nil {
				return err
			}

			var providers []ProviderItem
			if err := json.Unmarshal(data, &providers); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(providers)
			}

			headers := []string{"KEY", "NAME", "BASE URL", "FORMAT", "MODELS"}
			rows := make([][]string, len(providers))
			for i, p := range providers {
				rows[i] = []string{p.Key, p.Name, p.BaseURL, p.Format, joinSlice(p.Models)}
			}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newProviderAddCmd() *cobra.Command {
	var req CreateProviderReq
	var headers []string

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new provider",
		Example: "  ais admin provider add --key openai --name OpenAI --base-url https://api.openai.com --api-key sk-xxx --model gpt-4o --model gpt-4o-mini",
		RunE: func(cmd *cobra.Command, args []string) error {
			if req.Format == "" {
				req.Format = "chat"
			}
			if len(headers) > 0 {
				req.CustomHeaders = parseHeaderFlags(headers)
			}
			client := ClientFromCmd(cmd)
			data, err := client.Post(cmd.Context(), "/providers", req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&req.Key, "key", "", "Provider key (required)")
	cmd.Flags().StringVar(&req.Name, "name", "", "Display name (required)")
	cmd.Flags().StringVar(&req.BaseURL, "base-url", "", "Upstream base URL (required)")
	cmd.Flags().StringVar(&req.APIKey, "api-key", "", "Upstream API key (required)")
	cmd.Flags().StringVar(&req.Format, "format", "chat", "API format: chat, responses, anthropic")
	cmd.Flags().StringSliceVar(&req.Models, "model", nil, "Supported models (repeatable)")
	cmd.Flags().StringVar(&req.DefaultModel, "default-model", "", "Default model for auto-created route")
	cmd.Flags().BoolVar(&req.EnableProxy, "enable-proxy", false, "Route through global proxy")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "Custom upstream header 'Key: Value' (repeatable); overrides the forwarded client header")

	for _, f := range []string{"key", "name", "base-url", "api-key"} {
		_ = cmd.MarkFlagRequired(f)
	}
	bindOutputFlag(cmd)
	return cmd
}

func newProviderUpdateCmd() *cobra.Command {
	// Local vars for optional flags — only sent in request if explicitly set.
	var name, baseURL, apiKey, format string
	var models []string
	var enableProxy bool
	var headers []string

	cmd := &cobra.Command{
		Use:   "update <key>",
		Short: "Update a provider",
		Args:  needMinArgs(1, "update <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := url.PathEscape(args[0])

			// Build partial-update request: only include explicitly set flags.
			req := make(map[string]any)
			if cmd.Flags().Changed("name") {
				req["name"] = name
			}
			if cmd.Flags().Changed("base-url") {
				req["base_url"] = baseURL
			}
			if cmd.Flags().Changed("api-key") {
				req["api_key"] = apiKey
			}
			if cmd.Flags().Changed("format") {
				req["format"] = format
			}
			if cmd.Flags().Changed("model") {
				req["models"] = models
			}
			if cmd.Flags().Changed("enable-proxy") {
				req["enable_proxy"] = enableProxy
			}
			if cmd.Flags().Changed("header") {
				req["custom_headers"] = parseHeaderFlags(headers)
			}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/providers/"+key, req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Upstream base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Upstream API key")
	cmd.Flags().StringVar(&format, "format", "", "API format: chat, responses, anthropic")
	cmd.Flags().StringSliceVar(&models, "model", nil, "Supported models (repeatable, replaces all)")
	cmd.Flags().BoolVar(&enableProxy, "enable-proxy", false, "Route through global proxy")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "Custom upstream header 'Key: Value' (repeatable, replaces all); overrides the forwarded client header")
	bindOutputFlag(cmd)
	return cmd
}

func newProviderRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <key>",
		Short: "Remove a provider",
		Args:  needMinArgs(1, "remove <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			_, err := client.Delete(cmd.Context(), "/providers/"+url.PathEscape(args[0]))
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintMessage(fmt.Sprintf("provider %q removed", args[0]))
		},
	}
	bindOutputFlag(cmd)
	return cmd
}
