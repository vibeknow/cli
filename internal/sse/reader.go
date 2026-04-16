// Package sse provides a minimal Server-Sent Events reader per the W3C spec.
package sse

import (
	"bufio"
	"io"
	"strings"
)

type Event struct {
	ID    string
	Event string
	Data  string
}

type Reader struct {
	scanner *bufio.Scanner
}

func NewReader(r io.Reader) *Reader {
	return &Reader{scanner: bufio.NewScanner(r)}
}

// Next returns the next complete event. Returns io.EOF when the stream ends.
func (r *Reader) Next() (Event, error) {
	var ev Event
	var dataLines []string
	hasData := false

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if line == "" {
			if hasData || ev.Event != "" || ev.ID != "" {
				ev.Data = strings.Join(dataLines, "\n")
				return ev, nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "data":
			dataLines = append(dataLines, value)
			hasData = true
		case "event":
			ev.Event = value
		case "id":
			ev.ID = value
		}
	}

	if err := r.scanner.Err(); err != nil {
		return Event{}, err
	}

	if hasData || ev.Event != "" || ev.ID != "" {
		ev.Data = strings.Join(dataLines, "\n")
		return ev, nil
	}
	return Event{}, io.EOF
}
