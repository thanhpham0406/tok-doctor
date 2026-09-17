package source

import (
	"io"
	"strings"
	"testing"
)

func newLimitedRecordReader(r io.Reader, limit int) *RecordReader {
	reader := NewRecordReader(r)
	reader.limit = limit
	return reader
}

func TestRecordReaderReassemblesRecordLargerThanReadBuffer(t *testing.T) {
	payload := strings.Repeat("a", 512*1024)
	input := "{\"type\":\"first\"}\n{\"value\":\"" + payload + "\"}\n"

	reader := NewRecordReader(strings.NewReader(input))
	first, firstLine, oversized, err := reader.Next()
	if err != nil || oversized {
		t.Fatalf("first record: err=%v oversized=%v", err, oversized)
	}
	if string(first) != `{"type":"first"}` || firstLine != 1 {
		t.Fatalf("first record = %q at line %d", first, firstLine)
	}

	second, secondLine, oversized, err := reader.Next()
	if err != nil || oversized {
		t.Fatalf("second record: err=%v oversized=%v", err, oversized)
	}
	if !strings.Contains(string(second), payload) || secondLine != 2 {
		t.Fatalf("second record truncated at line %d: %d bytes", secondLine, len(second))
	}
}

func TestRecordReaderReportsOversizedRecordAndKeepsStreaming(t *testing.T) {
	input := "{\"value\":\"" + strings.Repeat("a", 4096) + "\"}\n{\"type\":\"after\"}\n"

	reader := newLimitedRecordReader(strings.NewReader(input), 128)
	raw, _, oversized, err := reader.Next()
	if err != nil {
		t.Fatalf("oversized record: %v", err)
	}
	if !oversized || raw != nil {
		t.Fatalf("raw=%q oversized=%v, want an oversized record without content", raw, oversized)
	}

	next, line, oversized, err := reader.Next()
	if err != nil || oversized {
		t.Fatalf("record after oversized: err=%v oversized=%v", err, oversized)
	}
	if string(next) != `{"type":"after"}` || line != 2 {
		t.Fatalf("record after oversized = %q at line %d, want the following record", next, line)
	}
}

func TestRecordReaderStopsAtEOFWithoutTrailingNewline(t *testing.T) {
	reader := NewRecordReader(strings.NewReader(`{"type":"last"}`))
	raw, line, oversized, err := reader.Next()
	if err != nil || oversized || line != 1 {
		t.Fatalf("last record: err=%v oversized=%v line=%d", err, oversized, line)
	}
	if string(raw) != `{"type":"last"}` {
		t.Fatalf("last record = %q", raw)
	}
	if _, _, _, err := reader.Next(); err != io.EOF {
		t.Fatalf("err after last record = %v, want io.EOF", err)
	}
}
