package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
)

type AdminHandler struct {
	provider *config.Provider
	mu       sync.Mutex
}

func NewAdminHandler(provider *config.Provider) *AdminHandler {
	return &AdminHandler{provider: provider}
}

func (a *AdminHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/admin/providers", a.listProviders)
	r.POST("/admin/providers", a.createProvider)
	r.PUT("/admin/providers/:key", a.updateProvider)
	r.DELETE("/admin/providers/:key", a.deleteProvider)

	r.GET("/admin/routes", a.listRoutes)
	r.POST("/admin/routes", a.createRoute)
	r.PUT("/admin/routes/:key", a.updateRoute)
	r.DELETE("/admin/routes/:key", a.deleteRoute)
	r.POST("/admin/routes/generate-key", a.generateKey)

	r.GET("/admin/presets", a.listPresets)
	r.GET("/admin/status", a.adminStatus)
	r.GET("/admin/apikeys/:type/:key", a.revealAPIKey)
}

func (a *AdminHandler) listProviders(c *gin.Context) {
	cfg := a.provider.Get()
	type providerItem struct {
		Key      string   `json:"key"`
		Name     string   `json:"name"`
		BaseURL  string   `json:"base_url"`
		Path     string   `json:"path"`
		APIKey   string   `json:"api_key"`
		Format   string   `json:"format"`
		LogoURL  string   `json:"logo_url"`
		Sponsor  bool     `json:"sponsor"`
		ThinkTag string   `json:"think_tag"`
		Models   []string `json:"models"`
	}

	items := make([]providerItem, 0, len(cfg.Providers))
	for k, p := range cfg.Providers {
		items = append(items, providerItem{
			Key:      k,
			Name:     p.Name,
			BaseURL:  p.BaseURL,
			Path:     p.Path,
			APIKey:   maskKey(p.APIKey),
			Format:   p.Format,
			LogoURL:  p.LogoURL,
			Sponsor:  p.Sponsor,
			ThinkTag: p.ThinkTag,
			Models:   p.Models,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (a *AdminHandler) createProvider(c *gin.Context) {
	var req struct {
		Key      string   `json:"key" binding:"required"`
		Name     string   `json:"name" binding:"required"`
		BaseURL  string   `json:"base_url" binding:"required"`
		Path     string   `json:"path"`
		APIKey   string   `json:"api_key" binding:"required"`
		Format   string   `json:"format"`
		LogoURL  string   `json:"logo_url"`
		Sponsor  bool     `json:"sponsor"`
		ThinkTag string   `json:"think_tag"`
		Models   []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Format == "" {
		req.Format = "chat"
	}
	if !config.ValidFormat(req.Format) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format, must be chat, responses, or anthropic"})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())
	if _, exists := cfg.Providers[req.Key]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("provider %q already exists", req.Key)})
		return
	}

	cfg.Providers[req.Key] = config.ProviderConfig{
		Name:     req.Name,
		BaseURL:  req.BaseURL,
		Path:     req.Path,
		APIKey:   req.APIKey,
		Format:   req.Format,
		LogoURL:  req.LogoURL,
		Sponsor:  req.Sponsor,
		ThinkTag: req.ThinkTag,
		Models:   req.Models,
	}

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"key": req.Key, "name": req.Name}})
}

func (a *AdminHandler) updateProvider(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Name     *string  `json:"name"`
		BaseURL  *string  `json:"base_url"`
		Path     *string  `json:"path"`
		APIKey   *string  `json:"api_key"`
		Format   *string  `json:"format"`
		LogoURL  *string  `json:"logo_url"`
		Sponsor  *bool    `json:"sponsor"`
		ThinkTag *string  `json:"think_tag"`
		Models   []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Format != nil && !config.ValidFormat(*req.Format) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format"})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())
	p, ok := cfg.Providers[key]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider %q not found", key)})
		return
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.BaseURL != nil {
		p.BaseURL = *req.BaseURL
	}
	if req.Path != nil {
		p.Path = *req.Path
	}
	if req.APIKey != nil && *req.APIKey != "" {
		p.APIKey = *req.APIKey
	}
	if req.Format != nil {
		p.Format = *req.Format
	}
	if req.LogoURL != nil {
		p.LogoURL = *req.LogoURL
	}
	if req.Sponsor != nil {
		p.Sponsor = *req.Sponsor
	}
	if req.ThinkTag != nil {
		p.ThinkTag = *req.ThinkTag
	}
	if req.Models != nil {
		p.Models = req.Models
	}

	cfg.Providers[key] = p

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"key": key}})
}

func (a *AdminHandler) deleteProvider(c *gin.Context) {
	key := c.Param("key")

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())
	if _, ok := cfg.Providers[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider %q not found", key)})
		return
	}

	delete(cfg.Providers, key)

	// Clear default_route if it references a route whose provider was deleted
	if cfg.DefaultRoute != "" {
		if dr, ok := cfg.Routes[cfg.DefaultRoute]; ok && dr.Provider == key {
			cfg.DefaultRoute = ""
		}
	}

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (a *AdminHandler) listRoutes(c *gin.Context) {
	cfg := a.provider.Get()
	type routeItem struct {
		Key                  string            `json:"key"`
		Provider             string            `json:"provider"`
		DefaultModel         string            `json:"default_model"`
		SceneMap             map[string]string `json:"scene_map"`
		ModelMap             map[string]string `json:"model_map"`
		LongContextThreshold int               `json:"long_context_threshold"`
	}

	items := make([]routeItem, 0, len(cfg.Routes))
	for k, r := range cfg.Routes {
		items = append(items, routeItem{
			Key:                  k,
			Provider:             r.Provider,
			DefaultModel:         r.DefaultModel,
			SceneMap:             r.SceneMap,
			ModelMap:             r.ModelMap,
			LongContextThreshold: r.LongContextThreshold,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (a *AdminHandler) createRoute(c *gin.Context) {
	var req struct {
		Key                  string            `json:"key" binding:"required"`
		Provider             string            `json:"provider" binding:"required"`
		DefaultModel         string            `json:"default_model"`
		SceneMap             map[string]string `json:"scene_map"`
		ModelMap             map[string]string `json:"model_map"`
		LongContextThreshold *int              `json:"long_context_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())

	if _, exists := cfg.Providers[req.Provider]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("provider %q not found", req.Provider)})
		return
	}

	if cfg.Routes == nil {
		cfg.Routes = make(map[string]config.RouteRule)
	}
	if _, exists := cfg.Routes[req.Key]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("route with key %q already exists", req.Key)})
		return
	}

	route := config.RouteRule{
		Provider:     req.Provider,
		DefaultModel: req.DefaultModel,
		SceneMap:     req.SceneMap,
		ModelMap:     req.ModelMap,
	}
	if req.LongContextThreshold != nil {
		route.LongContextThreshold = *req.LongContextThreshold
	}

	cfg.Routes[req.Key] = route

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"data": gin.H{"key": req.Key}}
	if w := validateRouteModels(cfg, route); len(w) > 0 {
		resp["warnings"] = w
	}
	c.JSON(http.StatusCreated, resp)
}

func (a *AdminHandler) updateRoute(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Provider             *string            `json:"provider"`
		DefaultModel         *string            `json:"default_model"`
		SceneMap             *map[string]string `json:"scene_map"`
		ModelMap             *map[string]string `json:"model_map"`
		LongContextThreshold *int               `json:"long_context_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())

	rule, ok := cfg.Routes[key]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("route %q not found", key)})
		return
	}

	if req.Provider != nil {
		if _, exists := cfg.Providers[*req.Provider]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("provider %q not found", *req.Provider)})
			return
		}
		rule.Provider = *req.Provider
	}
	if req.DefaultModel != nil {
		rule.DefaultModel = *req.DefaultModel
	}
	if req.SceneMap != nil {
		rule.SceneMap = *req.SceneMap
	}
	if req.ModelMap != nil {
		rule.ModelMap = *req.ModelMap
	}
	if req.LongContextThreshold != nil {
		rule.LongContextThreshold = *req.LongContextThreshold
	}

	cfg.Routes[key] = rule

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"data": gin.H{"key": key}}
	if w := validateRouteModels(cfg, rule); len(w) > 0 {
		resp["warnings"] = w
	}
	c.JSON(http.StatusOK, resp)
}

func (a *AdminHandler) deleteRoute(c *gin.Context) {
	key := c.Param("key")

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := copyConfig(a.provider.Get())

	if _, ok := cfg.Routes[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("route %q not found", key)})
		return
	}

	delete(cfg.Routes, key)

	if cfg.DefaultRoute == key {
		cfg.DefaultRoute = ""
	}

	if err := a.writeAndReload(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (a *AdminHandler) generateKey(c *gin.Context) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate key"})
		return
	}
	key := "gw-" + hex.EncodeToString(b)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"key": key}})
}

func (a *AdminHandler) listPresets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": config.ProviderPresets})
}

func (a *AdminHandler) adminStatus(c *gin.Context) {
	cfg := a.provider.Get()
	c.JSON(http.StatusOK, gin.H{
		"server":         cfg.Server,
		"default_route":  cfg.DefaultRoute,
		"provider_count": len(cfg.Providers),
		"route_count":    len(cfg.Routes),
	})
}

func (a *AdminHandler) revealAPIKey(c *gin.Context) {
	typ := c.Param("type")
	key := c.Param("key")
	reveal := c.Query("reveal") == "true"

	if !reveal {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reveal parameter required"})
		return
	}

	cfg := a.provider.Get()

	switch typ {
	case "provider":
		p, ok := cfg.Providers[key]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"api_key": p.APIKey}})
	case "route":
		_, ok := cfg.Routes[key]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"key": key}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be provider or route"})
	}
}

func (a *AdminHandler) writeAndReload(cfg *config.Config) error {
	if err := config.WriteConfig(a.provider.Path(), cfg); err != nil {
		return err
	}
	return a.provider.Reload()
}

func maskKey(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func copyConfig(cfg *config.Config) *config.Config {
	cp := &config.Config{
		Server:       cfg.Server,
		DefaultRoute: cfg.DefaultRoute,
		Providers:    make(map[string]config.ProviderConfig, len(cfg.Providers)),
		Routes:       make(map[string]config.RouteRule, len(cfg.Routes)),
	}
	for k, v := range cfg.Providers {
		cp.Providers[k] = v
	}
	for k, v := range cfg.Routes {
		sm := make(map[string]string, len(v.SceneMap))
		for sk, sv := range v.SceneMap {
			sm[sk] = sv
		}
		mm := make(map[string]string, len(v.ModelMap))
		for mk, mv := range v.ModelMap {
			mm[mk] = mv
		}
		cp.Routes[k] = config.RouteRule{
			Provider:             v.Provider,
			DefaultModel:         v.DefaultModel,
			SceneMap:             sm,
			ModelMap:             mm,
			LongContextThreshold: v.LongContextThreshold,
		}
	}
	return cp
}

// validateRouteModels checks if models referenced in a route exist in their
// respective provider's model list. Returns warnings for unrecognized models.
func validateRouteModels(cfg *config.Config, route config.RouteRule) []string {
	var warnings []string
	models := collectRouteModels(route)
	for provKey, modelNames := range models {
		p, ok := cfg.Providers[provKey]
		if !ok || len(p.Models) == 0 {
			continue
		}
		known := make(map[string]bool, len(p.Models))
		for _, m := range p.Models {
			known[m] = true
		}
		for _, m := range modelNames {
			if !known[m] {
				warnings = append(warnings, fmt.Sprintf("model %q not found in provider %q's model list", m, provKey))
			}
		}
	}
	return warnings
}

// collectRouteModels extracts all model references from a route rule,
// resolving provider:model format, grouped by provider key.
func collectRouteModels(route config.RouteRule) map[string][]string {
	result := make(map[string][]string)
	add := func(value string) {
		if value == "" {
			return
		}
		pk, model := config.SplitProviderModel(value, route.Provider)
		result[pk] = append(result[pk], model)
	}
	add(route.DefaultModel)
	for _, v := range route.SceneMap {
		add(v)
	}
	for _, v := range route.ModelMap {
		add(v)
	}
	return result
}
