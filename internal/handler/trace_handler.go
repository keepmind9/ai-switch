package handler

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	StartTime string
	EndTime   string
}

func (f traceFilters) matchTime(t string) bool {
	if t == "" {
		return true
	}
	if f.StartTime != "" && t < f.StartTime {
		return false
	}
	if f.EndTime != "" && t > f.EndTime {
		return false
	}
	return true
}

func (f traceFilters) match(s *traceSummary) bool {
	if !f.matchTime(s.Time) {
		return false
	}
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

// listTraces handles GET /api/admin/traces with cursor-based pagination.
func (t *TraceHandler) listTraces(c *gin.Context) {
	date := c.DefaultQuery("date", "")
	if date == "" {
		// Derive date from start_time if provided, otherwise use today
		if st := c.Query("start_time"); st != "" {
			if len(st) >= 10 {
				date = st[:10]
			}
		}
		if date == "" {
			date = currentDate()
		}
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	limit := pageSize + 1

	filters := traceFilters{
		Model:     c.Query("model"),
		Provider:  c.Query("provider"),
		SessionID: c.Query("session_id"),
		StartTime: strings.ReplaceAll(c.Query("start_time"), " ", "T"),
		EndTime:   strings.ReplaceAll(c.Query("end_time"), " ", "T"),
	}
	if s := c.Query("status"); s != "" {
		filters.Status, _ = strconv.Atoi(s)
	}

	// Decode cursor if present
	var cursor traceCursor
	if raw := c.Query("cursor"); raw != "" {
		if err := cursor.decode(raw); err != nil {
			sendFail(c, http.StatusBadRequest, CodeBadRequest, "invalid cursor")
			return
		}
	}

	filePath := logFilePath(t.dataDir, date)
	idxPath := indexFilePath(t.dataDir, date)

	// Try index file first, fall back to full scan for old data.
	var all []traceSummary
	var err error
	if _, statErr := os.Stat(idxPath); statErr == nil {
		all, err = scanIndex(idxPath, filters, limit)
	} else {
		all, err = scanTracesLegacy(filePath, filters, limit)
	}
	if err != nil {
		if os.IsNotExist(err) {
			sendFail(c, http.StatusNotFound, CodeNotFound, fmt.Sprintf("no logs for date %s", date))
			return
		}
		sendFail(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	// Apply cursor: skip items before or after the cursor position
	var start int
	if cursor.Time != "" {
		for i, g := range all {
			if g.Time == cursor.Time && g.AisReqID == cursor.AisReqID {
				if cursor.Dir == "prev" {
					start = i - pageSize
					if start < 0 {
						start = 0
					}
				} else {
					start = i + 1
				}
				break
			}
		}
	}

	if len(all) == 0 {
		sendOK(c, gin.H{
			"items":       []traceSummary{},
			"has_prev":    false,
			"has_next":    false,
			"prev_cursor": "",
			"next_cursor": "",
		})
		return
	}

	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	if start >= len(all) {
		start = len(all)
	}

	items := all[start:end]
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}

	hasPrev := start > 0
	hasNext := hasMore

	sendOK(c, gin.H{
		"items":    items,
		"has_prev": hasPrev,
		"has_next": hasNext,
		"prev_cursor": func() string {
			if !hasPrev || len(items) == 0 {
				return ""
			}
			first := items[0]
			return (&traceCursor{Time: first.Time, AisReqID: first.AisReqID, Dir: "prev"}).encode()
		}(),
		"next_cursor": func() string {
			if !hasNext || len(items) == 0 {
				return ""
			}
			last := items[len(items)-1]
			return (&traceCursor{Time: last.Time, AisReqID: last.AisReqID, Dir: "next"}).encode()
		}(),
	})
}

// traceCursor is an opaque pagination cursor.
type traceCursor struct {
	Time     string `json:"t"`
	AisReqID string `json:"r"`
	Dir      string `json:"d"`
}

func (c *traceCursor) encode() string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func (c *traceCursor) decode(s string) error {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, c)
}

// getTrace handles GET /api/admin/traces/:ais_req_id
func (t *TraceHandler) getTrace(c *gin.Context) {
	requestID := c.Param("ais_req_id")

	// Derive date from request ID (format: YYYYMMDDHHMMSSxxxxx)
	date := currentDate()
	if len(requestID) >= 8 {
		d := requestID[:4] + "-" + requestID[4:6] + "-" + requestID[6:8]
		if _, err := time.Parse("2006-01-02", d); err == nil {
			date = d
		}
	}

	filePath := logFilePath(t.dataDir, date)
	idxPath := indexFilePath(t.dataDir, date)

	var records []map[string]any
	var err error

	// Try index-based lookup first.
	if offset, lookupErr := lookupOffset(idxPath, requestID); lookupErr == nil && offset > 0 {
		records, err = scanTraceByOffset(filePath, requestID, offset)
	} else {
		records, err = scanTraceByIDLegacy(filePath, requestID)
	}
	if err != nil {
		if os.IsNotExist(err) {
			sendFail(c, http.StatusNotFound, CodeNotFound, "trace not found")
			return
		}
		sendFail(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	if len(records) == 0 {
		sendFail(c, http.StatusNotFound, CodeNotFound, "trace not found")
		return
	}

	sendOK(c, gin.H{
		"ais_req_id": requestID,
		"records":    records,
	})
}

func logFilePath(dataDir, date string) string {
	return filepath.Join(log.LogDir(dataDir), log.LLMLogFilePrefix+"-"+date+".log")
}

func indexFilePath(dataDir, date string) string {
	return filepath.Join(log.LogDir(dataDir), "llm-idx"+"-"+date+".log")
}

func currentDate() string {
	return currentClock.Now().Format("2006-01-02")
}

// clock abstraction for testing
var currentClock = struct{ Now func() time.Time }{Now: time.Now}

const maxScanLines = 200

// scanIndex reads the index file and returns filtered summaries sorted by time descending.
func scanIndex(filePath string, filters traceFilters, limit int) ([]traceSummary, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make([]traceSummary, 0)
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var s traceSummary
				if json.Unmarshal(line, &s) == nil {
					if filters.match(&s) {
						result = append(result, s)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time > result[j].Time
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// scanTraceByOffset seeks to offset in the JSONL file and collects records for requestID.
func scanTraceByOffset(filePath string, requestID string, offset int64) ([]map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	var records []map[string]any
	scanned := 0
	reader := bufio.NewReaderSize(f, 64*1024)
	for scanned < maxScanLines {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				scanned++
				var rec map[string]any
				if json.Unmarshal(line, &rec) == nil {
					rid, _ := rec["ais_req_id"].(string)
					if rid == requestID {
						records = append(records, rec)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	return records, nil
}

// lookupOffset searches the index file for a request_id and returns its offset.
func lookupOffset(filePath, requestID string) (int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var idx struct {
					AisReqID string `json:"ais_req_id"`
					Offset   int64  `json:"offset"`
				}
				if json.Unmarshal(line, &idx) == nil && idx.AisReqID == requestID {
					return idx.Offset, nil
				}
			}
		}
		if err != nil {
			break
		}
	}
	return 0, os.ErrNotExist
}

// scanTraces reads a JSONL trace file, groups records by request_id, applies filters,
// and returns up to limit summaries sorted by time descending.
func scanTracesLegacy(filePath string, filters traceFilters, limit int) ([]traceSummary, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	groups := make(map[string]*traceSummary)

	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var rec map[string]any
				if json.Unmarshal(line, &rec) == nil {
					rid, _ := rec["ais_req_id"].(string)
					if rid == "" {
						goto next
					}
					g, ok := groups[rid]
					if !ok {
						g = &traceSummary{AisReqID: rid}
						groups[rid] = g
					}
					mergeSummary(g, rec)
				}
			}
		}
	next:
		if err != nil {
			break
		}
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

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// scanTraceByID reads a JSONL trace file and returns all records for the given request_id.
func scanTraceByIDLegacy(filePath, requestID string) ([]map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []map[string]any

	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var rec map[string]any
				if json.Unmarshal(line, &rec) == nil {
					rid, _ := rec["ais_req_id"].(string)
					if rid == requestID {
						records = append(records, rec)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	return records, nil
}

func mergeSummary(g *traceSummary, rec map[string]any) {
	if v, ok := rec["session_id"].(string); ok && v != "" && g.SessionID == "" {
		g.SessionID = v
	}

	recType, _ := rec["type"].(string)

	switch recType {
	case "request":
		if v, ok := rec["time"].(string); ok && g.Time == "" {
			g.Time = v
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
