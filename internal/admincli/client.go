package admincli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIResponse mirrors the server's unified response envelope.
type APIResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// APIError is returned when the server responds with a non-zero code.
type APIError struct {
	StatusCode int
	Code       int
	Msg        string
}

func (e *APIError) Error() string { return fmt.Sprintf("API error %d: %s", e.Code, e.Msg) }

// AdminClient is an HTTP client for the ais admin API.
type AdminClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAdminClient creates a client targeting the given base URL (e.g. "http://127.0.0.1:12345/api").
func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

const adminPath = "/admin"

// doRequest sends an HTTP request and parses the unified response envelope.
func (c *AdminClient) doRequest(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	url := c.baseURL + adminPath + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to ais server at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, &APIError{StatusCode: resp.StatusCode, Code: apiResp.Code, Msg: apiResp.Msg}
	}
	return apiResp.Data, nil
}

// Get sends a GET request.
func (c *AdminClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil)
}

// Post sends a POST request with a JSON body.
func (c *AdminClient) Post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.doRequest(ctx, http.MethodPost, path, body)
}

// Put sends a PUT request with a JSON body.
func (c *AdminClient) Put(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.doRequest(ctx, http.MethodPut, path, body)
}

// Delete sends a DELETE request.
func (c *AdminClient) Delete(ctx context.Context, path string) (json.RawMessage, error) {
	return c.doRequest(ctx, http.MethodDelete, path, nil)
}
