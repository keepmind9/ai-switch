package e2e

import (
	"strings"
)

// sseEvent represents a single SSE event parsed from a stream.
type sseEvent struct {
	EventType string
	Data      string
}

// parseSSEStream parses raw SSE bytes into a list of events.
func parseSSEStream(body []byte) []sseEvent {
	var events []sseEvent
	var currentEvent string

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			events = append(events, sseEvent{
				EventType: currentEvent,
				Data:      data,
			})
			currentEvent = ""
		} else if strings.HasPrefix(line, "data:") {
			// Handle "data:value" without space
			data := strings.TrimPrefix(line, "data:")
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			events = append(events, sseEvent{
				EventType: currentEvent,
				Data:      data,
			})
			currentEvent = ""
		} else if line == "" {
			currentEvent = ""
		}
	}
	return events
}

// hasDataContaining checks if any SSE event data contains the given substring.
func hasDataContaining(events []sseEvent, substr string) bool {
	for _, e := range events {
		if strings.Contains(e.Data, substr) {
			return true
		}
	}
	return false
}

// hasEventWithType checks if any SSE event has the given event type.
func hasEventWithType(events []sseEvent, eventType string) bool {
	for _, e := range events {
		if e.EventType == eventType {
			return true
		}
	}
	return false
}

// hasDone checks if the stream contains a [DONE] marker.
func hasDone(events []sseEvent) bool {
	for _, e := range events {
		if strings.TrimSpace(e.Data) == "[DONE]" {
			return true
		}
	}
	return false
}
