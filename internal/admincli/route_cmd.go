package admincli

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// --- Route types mirroring API JSON shapes ---

// RouteItem is a single route in list output.
type RouteItem struct {
	Key                  string            `json:"key"`
	Provider             string            `json:"provider"`
	DefaultModel         string            `json:"default_model"`
	Disabled             bool              `json:"disabled"`
	SceneMap             map[string]string `json:"scene_map"`
	ModelMap             map[string]string `json:"model_map"`
	LongContextThreshold int               `json:"long_context_threshold"`
}

// CreateRouteReq is the request body for POST /admin/routes.
type CreateRouteReq struct {
	Key          string `json:"key"`
	Provider     string `json:"provider"`
	DefaultModel string `json:"default_model,omitempty"`
	Disabled     bool   `json:"disabled"`
}

func NewRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Manage routes",
	}

	cmd.AddCommand(
		newRouteListCmd(),
		newRouteAddCmd(),
		newRouteUpdateCmd(),
		newRouteRemoveCmd(),
		newRouteDefaultCmd(),
		newRouteEnableCmd(),
		newRouteDisableCmd(),
	)
	return cmd
}

func newRouteListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/routes")
			if err != nil {
				return err
			}

			var routes []RouteItem
			if err := json.Unmarshal(data, &routes); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(routes)
			}

			headers := []string{"KEY", "PROVIDER", "DEFAULT MODEL", "DISABLED"}
			rows := make([][]string, len(routes))
			for i, r := range routes {
				rows[i] = []string{r.Key, r.Provider, r.DefaultModel, fmt.Sprintf("%t", r.Disabled)}
			}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newRouteAddCmd() *cobra.Command {
	var req CreateRouteReq

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new route",
		Example: "  ais admin route add --key mykey --provider openai --default-model gpt-4o",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Post(cmd.Context(), "/routes", req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&req.Key, "key", "", "Route key (required)")
	cmd.Flags().StringVar(&req.Provider, "provider", "", "Provider key (required)")
	cmd.Flags().StringVar(&req.DefaultModel, "default-model", "", "Default model")
	cmd.Flags().BoolVar(&req.Disabled, "disabled", false, "Disable this route")

	for _, f := range []string{"key", "provider"} {
		_ = cmd.MarkFlagRequired(f)
	}
	bindOutputFlag(cmd)
	return cmd
}

func newRouteUpdateCmd() *cobra.Command {
	var provider, defaultModel string
	var disabled bool

	cmd := &cobra.Command{
		Use:   "update <key>",
		Short: "Update a route",
		Args:  needMinArgs(1, "update <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := url.PathEscape(args[0])
			req := make(map[string]any)
			if cmd.Flags().Changed("provider") {
				req["provider"] = provider
			}
			if cmd.Flags().Changed("default-model") {
				req["default_model"] = defaultModel
			}
			if cmd.Flags().Changed("disabled") {
				req["disabled"] = disabled
			}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/routes/"+key, req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Provider key")
	cmd.Flags().StringVar(&defaultModel, "default-model", "", "Default model")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Disable this route")
	bindOutputFlag(cmd)
	return cmd
}

func newRouteRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <key>",
		Short: "Remove a route",
		Args:  needMinArgs(1, "remove <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			_, err := client.Delete(cmd.Context(), "/routes/"+url.PathEscape(args[0]))
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintMessage(fmt.Sprintf("route %q removed", args[0]))
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

// newRouteDefaultCmd creates the "route default" command group.
func newRouteDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default",
		Short: "Manage default routes",
	}
	cmd.AddCommand(
		newRouteDefaultListCmd(),
		newRouteDefaultSetCmd(),
		newRouteDefaultRemoveCmd(),
	)
	return cmd
}

// DefaultRouteItem mirrors the default route fields in status API response.
type DefaultRouteItem struct {
	DefaultRoute          string `json:"default_route"`
	DefaultAnthropicRoute string `json:"default_anthropic_route"`
	DefaultResponsesRoute string `json:"default_responses_route"`
	DefaultChatRoute      string `json:"default_chat_route"`
}

const routeKeyHelp = `Available route keys can be listed with "route list".`

func newRouteDefaultListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show current default routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := ClientFromCmd(cmd)
			data, err := client.Get(cmd.Context(), "/status")
			if err != nil {
				return err
			}

			var raw struct {
				DefaultRoute          string `json:"default_route"`
				DefaultAnthropicRoute string `json:"default_anthropic_route"`
				DefaultResponsesRoute string `json:"default_responses_route"`
				DefaultChatRoute      string `json:"default_chat_route"`
			}
			if err := json.Unmarshal(data, &raw); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			f := FormatterFromCmd(cmd)
			if f.Format == FormatJSON {
				return f.PrintJSON(DefaultRouteItem{
					DefaultRoute:          raw.DefaultRoute,
					DefaultAnthropicRoute: raw.DefaultAnthropicRoute,
					DefaultResponsesRoute: raw.DefaultResponsesRoute,
					DefaultChatRoute:      raw.DefaultChatRoute,
				})
			}

			headers := []string{"TYPE", "ROUTE KEY"}
			rows := [][]string{
				{"default", dashIfEmpty(raw.DefaultRoute)},
				{"anthropic", dashIfEmpty(raw.DefaultAnthropicRoute)},
				{"responses", dashIfEmpty(raw.DefaultResponsesRoute)},
				{"chat", dashIfEmpty(raw.DefaultChatRoute)},
			}
			return f.PrintTable(headers, rows)
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newRouteDefaultSetCmd() *cobra.Command {
	var defaultRoute, anthropic, responses, chat string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set default routes",
		Long: routeKeyHelp + "\n\n" +
			"Set a specific protocol's default route to the given route key.",
		Example: "  ais admin route default set --default mykey --anthropic claude-key",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := make(map[string]any)
			if cmd.Flags().Changed("default") {
				req["default_route"] = defaultRoute
			}
			if cmd.Flags().Changed("anthropic") {
				req["default_anthropic_route"] = anthropic
			}
			if cmd.Flags().Changed("responses") {
				req["default_responses_route"] = responses
			}
			if cmd.Flags().Changed("chat") {
				req["default_chat_route"] = chat
			}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/default-routes", req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().StringVar(&defaultRoute, "default", "", "Global default route key")
	cmd.Flags().StringVar(&anthropic, "anthropic", "", "Default route for Anthropic protocol")
	cmd.Flags().StringVar(&responses, "responses", "", "Default route for Responses protocol")
	cmd.Flags().StringVar(&chat, "chat", "", "Default route for Chat protocol")
	bindOutputFlag(cmd)
	return cmd
}

func newRouteDefaultRemoveCmd() *cobra.Command {
	var defaultRoute, anthropic, responses, chat bool

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove default routes (reset to empty)",
		Long: routeKeyHelp + "\n\n" +
			"Clear the selected default route(s) by setting them to empty.",
		Example: "  ais admin route default remove --anthropic\n" +
			"  ais admin route default remove --default --chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := make(map[string]any)
			if defaultRoute {
				req["default_route"] = ""
			}
			if anthropic {
				req["default_anthropic_route"] = ""
			}
			if responses {
				req["default_responses_route"] = ""
			}
			if chat {
				req["default_chat_route"] = ""
			}

			if len(req) == 0 {
				return fmt.Errorf("specify at least one flag: --default, --anthropic, --responses, --chat")
			}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/default-routes", req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}

	cmd.Flags().BoolVar(&defaultRoute, "default", false, "Remove global default route")
	cmd.Flags().BoolVar(&anthropic, "anthropic", false, "Remove Anthropic default route")
	cmd.Flags().BoolVar(&responses, "responses", false, "Remove Responses default route")
	cmd.Flags().BoolVar(&chat, "chat", false, "Remove Chat default route")
	bindOutputFlag(cmd)
	return cmd
}

func newRouteEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <key>",
		Short: "Enable a route",
		Args:  needMinArgs(1, "enable <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := url.PathEscape(args[0])
			req := map[string]any{"disabled": false}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/routes/"+key, req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}
	bindOutputFlag(cmd)
	return cmd
}

func newRouteDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <key>",
		Short: "Disable a route",
		Args:  needMinArgs(1, "disable <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := url.PathEscape(args[0])
			req := map[string]any{"disabled": true}

			client := ClientFromCmd(cmd)
			data, err := client.Put(cmd.Context(), "/routes/"+key, req)
			if err != nil {
				return err
			}
			f := FormatterFromCmd(cmd)
			return f.PrintJSON(json.RawMessage(data))
		},
	}
	bindOutputFlag(cmd)
	return cmd
}
