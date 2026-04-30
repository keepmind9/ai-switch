package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/log"
)

// TraceHandler provides REST API endpoints for querying JSONL trace files.
type TraceHandler struct {
	dataDir string
}

// NewTraceHandler creates a new TraceHandler with the given data directory.
func NewTraceHandler(dataDir string) *TraceHandler {
	return &TraceHandler{dataDir: dataDir}
}

// RegisterRoutes registers trace API routes on the given router group.
func (t *TraceHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/admin/traces/dates", t.listDates)
	r.GET("/admin/traces/:ais_req_id", t.getTrace)
	r.GET("/admin/traces", t.listTraces)
}

// traceSummary is the merged summary for a single request shown in list view.
type traceSummary struct {
	AisReqID       string `json:"ais_req_id"`
	Time           string `json:"time"`
	SessionID      string `json:"session_id,omitempty"`
	ClientProtocol string `json:"client_protocol,omitempty"`
	Model          string `json:"model,omitempty"`
	Stream         bool   `json:"stream,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Status         int    `json:"status,omitempty"`
	LatencyMs      int64  `json:"latency_ms,omitempty"`
	InputTokens    int64  `json:"input_tokens,omitempty"`
	OutputTokens   int64  `json:"output_tokens,omitempty"`
}

// traceFilters holds optional query filters for the list endpoint.
type traceFilters struct {
	Model     string
	Provider  string
	Status    int
	SessionID string
}

func (f traceFilters) match(s *traceSummary) bool {
	if f.Model != "" && !strings.Contains(strings.ToLower(s.Model), strings.ToLower(f.Model)) {
		return false
	}
	if f.Provider != "" && !strings.Contains(strings.ToLower(s.Provider), strings.ToLower(f.Provider)) {
		return false
	}
	if f.Status != 0 && s.Status != f.Status {
		return false
	}
	if f.SessionID != "" && s.SessionID != f.SessionID {
		return false
	}
	return true
}

// listTraces handles GET /api/admin/traces
func (t *TraceHandler) listTraces(c *gin.Context) {
	date := c.DefaultQuery("date", "")
	if date == "" {
		date = currentDate()
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	filters := traceFilters{
		Model:     c.Query("model"),
		Provider:  c.Query("provider"),
		SessionID: c.Query("session_id"),
	}
	if s := c.Query("status"); s != "" {
		filters.Status, _ = strconv.Atoi(s)
	}

	filePath := logFilePath(t.dataDir, date)
	groups, err := scanTraces(filePath, filters, false)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no logs for date %s", date)})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	total := len(groups)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"items":     groups[start:end],
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// getTrace handles GET /api/admin/traces/:request_id
func (t *TraceHandler) getTrace(c *gin.Context) {
	requestID := c.Param("ais_req_id")
	date := c.DefaultQuery("date", "")
	if date == "" {
		date = currentDate()
	}

	filePath := logFilePath(t.dataDir, date)
	records, err := scanTraceByID(filePath, requestID)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"ais_req_id": requestID,
			"records":    records,
		},
	})
}

// listDates handles GET /api/admin/traces/dates
func (t *TraceHandler) listDates(c *gin.Context) {
	files, err := log.ListLogFiles(t.dataDir, log.LLMLogFilePrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	prefix := log.LLMLogFilePrefix + "-"
	var dates []string
	for _, f := range files {
		name := strings.TrimPrefix(f.Name(), prefix)
		name = strings.TrimSuffix(name, ".log")
		dates = append(dates, name)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i] > dates[j]
	})

	c.JSON(http.StatusOK, gin.H{"data": dates})
}

func logFilePath(dataDir, date string) string {
	return filepath.Join(log.LogDir(dataDir), log.LLMLogFilePrefix+"-"+date+".log")
}

func currentDate() string {
	return currentClock.Now().Format("2006-01-02")
}

// clock abstraction for testing
var currentClock = struct{ Now func() time.Time }{Now: time.Now}

// scanTraces reads a JSONL trace file, groups records by request_id, applies filters,
// and returns summaries sorted by time descending. When includeBody is false, body is omitted.
func scanTraces(filePath string, filters traceFilters, includeBody bool) ([]traceSummary, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	groups := make(map[string]*traceSummary)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		rid, _ := rec["ais_req_id"].(string)
		if rid == "" {
			continue
		}

		g, ok := groups[rid]
		if !ok {
			g = &traceSummary{AisReqID: rid}
			groups[rid] = g
		}

		mergeSummary(g, rec)
	}

	result := make([]traceSummary, 0, len(groups))
	for _, g := range groups {
		if !filters.match(g) {
			continue
		}
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time > result[j].Time
	})

	return result, nil
}

// scanTraceByID reads a JSONL trace file and returns all records for the given request_id.
func scanTraceByID(filePath, requestID string) ([]map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []map[string]any

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		rid, _ := rec["ais_req_id"].(string)
		if rid == requestID {
			records = append(records, rec)
		}
	}

	return records, nil
}

func mergeSummary(g *traceSummary, rec map[string]any) {
	recType, _ := rec["type"].(string)

	switch recType {
	case "request":
		if v, ok := rec["time"].(string); ok && g.Time == "" {
			g.Time = v
		}
		if v, ok := rec["session_id"].(string); ok && v != "" {
			g.SessionID = v
		}
		if v, ok := rec["client_protocol"].(string); ok {
			g.ClientProtocol = v
		}
		if v, ok := rec["model"].(string); ok && g.Model == "" {
			g.Model = v
		}
		if v, ok := rec["stream"].(bool); ok {
			g.Stream = v
		}
	case "upstream_req":
		if v, ok := rec["provider"].(string); ok && g.Provider == "" {
			g.Provider = v
		}
	case "upstream_resp":
		if v, ok := rec["status"].(float64); ok {
			g.Status = int(v)
		}
		if v, ok := rec["latency_ms"].(float64); ok {
			g.LatencyMs = int64(v)
		}
	case "response":
		if v, ok := rec["model"].(string); ok && v != "" {
			g.Model = v
		}
		if v, ok := rec["provider"].(string); ok && v != "" {
			g.Provider = v
		}
		if v, ok := rec["input_tokens"].(float64); ok {
			g.InputTokens = int64(v)
		}
		if v, ok := rec["output_tokens"].(float64); ok {
			g.OutputTokens = int64(v)
		}
	}
}
