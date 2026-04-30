# Trace Viewer Frontend Requirements

## Overview

A single-page trace viewer embedded in the ai-switch admin UI at `/ui/traces`. Displays JSONL trace records from the backend API, grouped by request_id, for debugging LLM proxy requests.

## Backend API

All endpoints are under `/api/admin/traces` (localhost-only).

### GET /api/admin/traces

List traces grouped by request_id. No body fields in response.

**Query params:**

| Param     | Type   | Default        | Description                          |
|-----------|--------|----------------|--------------------------------------|
| `date`    | string | today          | Date in `YYYY-MM-DD` format          |
| `model`   | string | -              | Substring match on model name        |
| `provider`| string | -              | Substring match on provider key      |
| `status`  | int    | -              | Exact match on upstream HTTP status   |
| `page`    | int    | 1              | Page number (1-based)                |
| `page_size`| int   | 20             | Items per page (max 100)             |

**Response:**

```json
{
  "data": {
    "items": [
      {
        "request_id": "20260430075235a3f1b2c",
        "time": "2026-04-30T07:52:35.123+08:00",
        "client_protocol": "anthropic",
        "model": "claude-sonnet-4-6",
        "stream": true,
        "provider": "minimax",
        "status": 200,
        "latency_ms": 1234,
        "input_tokens": 100,
        "output_tokens": 50
      }
    ],
    "total": 42,
    "page": 1,
    "page_size": 20
  }
}
```

### GET /api/admin/traces/:request_id

Get full detail for a single trace (all records with body).

**Query params:**

| Param  | Type   | Default | Description                    |
|--------|--------|---------|--------------------------------|
| `date` | string | today   | Hint for which log file to search |

**Response:**

```json
{
  "data": {
    "request_id": "20260430075235a3f1b2c",
    "records": [
      {
        "type": "request",
        "time": "2026-04-30T07:52:35.123+08:00",
        "client_protocol": "anthropic",
        "model": "claude-sonnet-4-6",
        "stream": true,
        "body": "{...full JSON...}"
      },
      {
        "type": "upstream_req",
        "time": "2026-04-30T07:52:35.156+08:00",
        "upstream_protocol": "chat",
        "model": "claude-sonnet-4-6",
        "provider": "minimax",
        "url": "https://api.minimax.chat/v1/chat",
        "body": "{...full JSON...}"
      },
      {
        "type": "upstream_resp",
        "time": "2026-04-30T07:52:36.390+08:00",
        "status": 200,
        "latency_ms": 1234,
        "provider": "minimax",
        "url": "https://api.minimax.chat/v1/chat",
        "body": "...full response body..."
      },
      {
        "type": "response",
        "time": "2026-04-30T07:52:36.456+08:00",
        "model": "claude-sonnet-4-6",
        "provider": "minimax",
        "input_tokens": 100,
        "output_tokens": 50,
        "body": "...full response body..."
      }
    ]
  }
}
```

### GET /api/admin/traces/dates

List available trace dates for date picker.

**Response:**

```json
{
  "data": ["2026-04-30", "2026-04-29", "2026-04-28"]
}
```

Sorted descending (newest first).

## UI Requirements

### 1. Trace List Page

- **Date picker**: dropdown populated from `/api/admin/traces/dates`, default to today
- **Filters**: text inputs for model and provider, dropdown for status (200, 400, 500, all)
- **Table columns**: Time, Request ID (clickable), Protocol, Model, Provider, Status (color-coded: green=2xx, red=4xx/5xx), Latency, Tokens (input/output), Stream (badge)
- **Pagination**: page controls at bottom, show total count
- **Sorting**: default by time descending

### 2. Trace Detail Page (expand or new view)

When clicking a request_id in the list, show all 4 records as a timeline:

```
[request] ──> [upstream_req] ──> [upstream_resp] ──> [response]
  07:52:35       07:52:35           07:52:36           07:52:36
```

Each record card shows:
- Record type badge (colored by type)
- All non-body fields as key-value pairs
- Body field in a collapsible JSON viewer (syntax highlighted, collapsible nodes)

### 3. Design Notes

- This is a developer debugging tool, prioritize information density over aesthetics
- JSON body can be very large (streaming SSE content), use virtual scrolling or lazy rendering
- Status codes: 2xx = green badge, 4xx = yellow, 5xx = red
- Stream field: show as a small badge (SSE/non-SSE)
- Latency: format as "1.2s" or "234ms"
- Use the existing admin UI style (if any) or keep it minimal
