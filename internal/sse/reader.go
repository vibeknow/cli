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

// maxLineBytes caps one SSE line. bufio.Scanner's own default is 64KB, which
// is not enough: a terminal event carries the whole rendered package —
// every scene's layout code — on one `data:` line, and a long video clears
// 64KB easily. The failure mode that produced this limit is worth naming,
// because it is not obviously a size problem: Scanner returns ErrTooLong,
// the stream ends, and the run reads as "the backend went quiet" on the one
// event that says the work is finished.
const maxLineBytes = 8 << 20

func NewReader(r io.Reader) *Reader {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	return &Reader{scanner: s}
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
