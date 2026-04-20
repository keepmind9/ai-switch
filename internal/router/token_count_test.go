package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountTokens_BasicMessage(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"Hello, how are you?"}]}`
	count := countTokens([]byte(body))
	assert.Greater(t, count, 0)
}

func TestCountTokens_SystemPrompt(t *testing.T) {
	body := `{"system":"You are a helpful assistant.","messages":[{"role":"user","content":"Hi"}]}`
	count := countTokens([]byte(body))
	assert.Greater(t, count, 0)
}

func TestCountTokens_ArrayContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"What is in this image?"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}]}]}`
	count := countTokens([]byte(body))
	assert.Greater(t, count, 0)
}

func TestCountTokens_Tools(t *testing.T) {
	body := `{"messages":[],"tools":[{"name":"web_search","description":"Search the web","input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}]}`
	count := countTokens([]byte(body))
	assert.Greater(t, count, 0)
}

func TestCountTokens_EmptyBody(t *testing.T) {
	count := countTokens([]byte(`{}`))
	assert.Equal(t, 0, count)
}

func TestCountTokens_InvalidJSON(t *testing.T) {
	count := countTokens([]byte(`not json`))
	assert.Equal(t, 0, count)
}

func TestCountTokens_MoreTextMoreTokens(t *testing.T) {
	short := `{"messages":[{"role":"user","content":"Hi"}]}`
	long := `{"messages":[{"role":"user","content":"This is a much longer message with many more words to encode, which should definitely result in a higher token count than the short message above."}]}`
	shortCount := countTokens([]byte(short))
	longCount := countTokens([]byte(long))
	assert.Greater(t, longCount, shortCount)
}
