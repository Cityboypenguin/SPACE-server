package sse

import (
	"encoding/json"
	"fmt"
	"io"
)

// writeSSE writes an SSE-formatted message.
func writeSSE(w io.Writer, ev Event) error {
	// IDの書き込み
	if ev.ID != 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", ev.ID); err != nil {
			return err
		}
	}

	// Event Typeの書き込み
	if ev.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
			return err
		}
	}

	// Data is encoded as JSON for predictable client parsing.
	if ev.Data != nil {
		jsonData, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n", jsonData); err != nil {
			return err
		}
	}

	// Message terminator.
	_, err := fmt.Fprint(w, "\n")
	return err
}
