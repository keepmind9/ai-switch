package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// Gemini request/response types.

type GeminiRequest struct {
	Contents          []GeminiContent         `json:"contents,omitempty"`
	SystemInstruction *GeminiContent          `json:"system_instruction,omitempty"`
	Tools             []GeminiToolDeclaration `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"tool_config,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text         string          `json:"text,omitempty"`
	FunctionCall *GeminiFuncCall `json:"functionCall,omitempty"`
	FunctionResp *GeminiFuncResp `json:"functionResponse,omitempty"`
}

type GeminiFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type GeminiFuncResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

type GeminiToolDeclaration struct {
	FunctionDeclarations []GeminiFuncDecl `json:"functionDeclarations,omitempty"`
}

type GeminiFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFuncCallingConfig `json:"function_calling_config,omitempty"`
}

type GeminiFuncCallingConfig struct {
	Mode string `json:"mode,omitempty"`
}

type GeminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
}

// Gemini response types.

type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates,omitempty"`
	UsageMetadata *GeminiUsageMeta  `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

type GeminiCandidate struct {
	Content      *GeminiContent `json:"content,omitempty"`
	FinishReason string         `json:"finishReason,omitempty"`
}

type GeminiUsageMeta struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
}

// --- Chat → Gemini request conversion ---

// ChatToGeminiRequest converts a Chat Completions request to a Gemini generateContent request.
func (c *Converter) ChatToGeminiRequest(req *types.ChatRequest) (*GeminiRequest, error) {
	gemReq := &GeminiRequest{}

	// Generation config
	gc := &GeminiGenerationConfig{}
	hasConfig := false
	if req.MaxTokens > 0 {
		gc.MaxOutputTokens = req.MaxTokens
		hasConfig = true
	}
	if req.Temperature > 0 {
		gc.Temperature = req.Temperature
		hasConfig = true
	}
	if req.TopP > 0 {
		gc.TopP = req.TopP
		hasConfig = true
	}
	if hasConfig {
		gemReq.GenerationConfig = gc
	}

	// Tools
	if len(req.Tools) > 0 {
		var decls []GeminiFuncDecl
		for _, t := range req.Tools {
			decls = append(decls, GeminiFuncDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		gemReq.Tools = []GeminiToolDeclaration{{FunctionDeclarations: decls}}

		// Tool choice
		gemReq.ToolConfig = chatToolChoiceToGemini(req.ToolChoice)
	}

	// Build toolCallID -> functionName lookup for tool result mapping
	toolCallNames := map[string]string{}
	for _, msg := range req.Messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				toolCallNames[tc.ID] = tc.Function.Name
			}
		}
	}

	// Messages
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			gemReq.SystemInstruction = &GeminiContent{
				Parts: []GeminiPart{{Text: derefStr(msg.Content)}},
			}
		case "tool":
			funcName := toolCallNames[msg.ToolCallID]
			if funcName == "" {
				funcName = msg.ToolCallID
			}
			gemReq.Contents = append(gemReq.Contents, GeminiContent{
				Role: "user",
				Parts: []GeminiPart{{
					FunctionResp: &GeminiFuncResp{
						Name: funcName,
						Response: map[string]any{
							"result": derefStr(msg.Content),
						},
					},
				}},
			})
		case "assistant":
			content := gemReq.Contents
			if len(msg.ToolCalls) > 0 {
				var parts []GeminiPart
				if msg.Content != nil && *msg.Content != "" {
					parts = append(parts, GeminiPart{Text: *msg.Content})
				}
				for _, tc := range msg.ToolCalls {
					args := make(map[string]any)
					if tc.Function.Arguments != "" {
						json.Unmarshal([]byte(tc.Function.Arguments), &args)
					}
					parts = append(parts, GeminiPart{
						FunctionCall: &GeminiFuncCall{
							Name: tc.Function.Name,
							Args: args,
						},
					})
				}
				content = append(content, GeminiContent{Role: "model", Parts: parts})
			} else if msg.Content != nil {
				content = append(content, GeminiContent{
					Role:  "model",
					Parts: []GeminiPart{{Text: *msg.Content}},
				})
			}
			gemReq.Contents = content
		default: // user
			gemReq.Contents = append(gemReq.Contents, GeminiContent{
				Role:  "user",
				Parts: []GeminiPart{{Text: derefStr(msg.Content)}},
			})
		}
	}

	return gemReq, nil
}

// GeminiResponseToChat converts a Gemini response to a Chat Completions response.
func (c *Converter) GeminiResponseToChat(gemResp *GeminiResponse, model string) (*types.ChatResponse, error) {
	chatResp := &types.ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
	}

	if len(gemResp.Candidates) > 0 && gemResp.Candidates[0].Content != nil {
		var textParts []string
		var toolCalls []types.ToolCall
		for i, part := range gemResp.Candidates[0].Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, types.ToolCall{
					ID:   fmt.Sprintf("call_%d", i),
					Type: "function",
					Function: types.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
		msg := types.ChatMessage{Role: "assistant", Content: strPtr(strings.Join(textParts, ""))}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		chatResp.Choices = append(chatResp.Choices, types.ChatChoice{
			Index:        0,
			Message:      msg,
			FinishReason: geminiFinishToChat(gemResp.Candidates[0].FinishReason, len(toolCalls) > 0),
		})
	}

	if chatResp.Choices == nil {
		chatResp.Choices = append(chatResp.Choices, types.ChatChoice{
			Index: 0, Message: types.ChatMessage{Role: "assistant", Content: strPtr("")}, FinishReason: "stop",
		})
	}

	usage := types.ChatUsage{}
	if gemResp.UsageMetadata != nil {
		usage.PromptTokens = gemResp.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = gemResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = gemResp.UsageMetadata.TotalTokenCount
	}
	chatResp.Usage = usage

	return chatResp, nil
}

// --- Gemini request → Responses request (for clients sending Responses to Gemini upstream) ---

// ResponsesToGeminiRequest converts a Responses API request to a Gemini request.
func (c *Converter) ResponsesToGeminiRequest(req *types.ResponsesRequest) (*GeminiRequest, error) {
	chatReq, err := c.ResponsesToChat(req)
	if err != nil {
		return nil, err
	}
	return c.ChatToGeminiRequest(chatReq)
}

// AnthropicToGeminiRequest converts an Anthropic request to a Gemini request.
func (c *Converter) AnthropicToGeminiRequest(req *AnthropicRequest) (*GeminiRequest, error) {
	chatReq, err := c.AnthropicToChat(req)
	if err != nil {
		return nil, err
	}
	return c.ChatToGeminiRequest(chatReq)
}

// GeminiResponseToResponses converts a Gemini response to a Responses API response.
func (c *Converter) GeminiResponseToResponses(gemResp *GeminiResponse, model, thinkTag string) (*types.ResponsesResponse, error) {
	chatResp, err := c.GeminiResponseToChat(gemResp, model)
	if err != nil {
		return nil, err
	}
	return c.ChatToResponses(chatResp, model, thinkTag)
}

// GeminiResponseToAnthropic converts a Gemini response to an Anthropic response.
func (c *Converter) GeminiResponseToAnthropic(gemResp *GeminiResponse, model, thinkTag string) (*AnthropicResponse, error) {
	chatResp, err := c.GeminiResponseToChat(gemResp, model)
	if err != nil {
		return nil, err
	}
	return c.ChatToAnthropic(chatResp, model, thinkTag)
}

// --- Helper functions ---

func geminiFinishToChat(reason string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return "stop"
	}
}

func chatToolChoiceToGemini(choice any) *GeminiToolConfig {
	if choice == nil {
		return nil
	}
	switch v := choice.(type) {
	case string:
		switch v {
		case "auto":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFuncCallingConfig{Mode: "AUTO"}}
		case "required":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFuncCallingConfig{Mode: "ANY"}}
		case "none":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFuncCallingConfig{Mode: "NONE"}}
		}
	case map[string]any:
		// function-specific choice not directly supported in Gemini, use AUTO
		return &GeminiToolConfig{FunctionCallingConfig: &GeminiFuncCallingConfig{Mode: "AUTO"}}
	}
	return nil
}
