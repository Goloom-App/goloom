package sse

import (
	"io"
	"strconv"
	"strings"
)

type Event struct {
	ID    string
	Type  string
	Data  string
	Retry int
}

func (e Event) Write(w io.Writer) error {
	var b strings.Builder
	if e.ID != "" {
		b.WriteString("id: ")
		b.WriteString(e.ID)
		b.WriteByte('\n')
	}
	if e.Type != "" {
		b.WriteString("event: ")
		b.WriteString(e.Type)
		b.WriteByte('\n')
	}
	if e.Retry > 0 {
		b.WriteString("retry: ")
		b.WriteString(strconv.Itoa(e.Retry))
		b.WriteByte('\n')
	}
	for line := range strings.SplitSeq(e.Data, "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	_, err := io.WriteString(w, b.String())
	return err
}
