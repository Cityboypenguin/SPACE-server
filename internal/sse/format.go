package sse

import (
	"encoding/json"
	"fmt"
	"io"
)

func writeSSE(w io.Writer, ev Event) error {
	if ev.ID != 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", ev.ID); err != nil {
			return err
		}
	}
	if ev.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
			return err
		}
	}
	if ev.Data != nil {
		jsonData, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n", jsonData); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}
