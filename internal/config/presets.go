package config

type ProviderPreset struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Format  string `json:"format"`
	LogoURL string `json:"logo_url"`
}

var ProviderPresets = []ProviderPreset{
	{Key: "minimax", Name: "MiniMax", BaseURL: "https://api.minimaxi.com", Format: "chat"},
	{Key: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com", Format: "chat"},
	{Key: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com", Format: "chat"},
	{Key: "anthropic", Name: "Anthropic", BaseURL: "https://api.anthropic.com", Format: "anthropic"},
	{Key: "zhipu", Name: "Zhipu", BaseURL: "https://open.bigmodel.cn/api/anthropic", Format: "anthropic"},
	{Key: "gemini", Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com", Format: "chat"},
	{Key: "moonshot", Name: "Moonshot", BaseURL: "https://api.moonshot.cn", Format: "chat"},
	{Key: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api", Format: "chat"},
}
