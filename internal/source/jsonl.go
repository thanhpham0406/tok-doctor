package source

import (
	"bufio"
	"io"
)

// MaxRecordBytes bounds a single JSONL record. It is a safety valve against
// pathological input, not a functional limit: a record above the bound is
// consumed and reported as oversized so the rest of the file stays readable.
const MaxRecordBytes = 64 * 1024 * 1024

// RecordReader streams newline-delimited JSON records. Memory grows with one
// record, never with the file, and a record longer than the read buffer is
// assembled instead of failing the stream.
type RecordReader struct {
	reader *bufio.Reader
	buf    []byte
	line   int
	limit  int
}

func NewRecordReader(r io.Reader) *RecordReader {
	return &RecordReader{reader: bufio.NewReaderSize(r, 64*1024), limit: MaxRecordBytes}
}

// Next returns the next record without its trailing newline. The returned
// slice is only valid until the following call. A record larger than
// MaxRecordBytes is consumed and reported through oversized with a nil raw.
func (rr *RecordReader) Next() (raw []byte, line int, oversized bool, err error) {
	rr.buf = rr.buf[:0]
	terminated := false
	for !terminated {
		chunk, readErr := rr.reader.ReadSlice('\n')
		rr.buf = append(rr.buf, chunk...)
		switch readErr {
		case nil, io.EOF:
			terminated = true
		case bufio.ErrBufferFull:
		default:
			return nil, rr.line, false, readErr
		}
		if len(rr.buf) > rr.limit {
			if terminated {
				rr.line++
				rr.buf = nil
				return nil, rr.line, true, nil
			}
			return rr.discard()
		}
	}
	if len(rr.buf) == 0 {
		return nil, rr.line, false, io.EOF
	}
	rr.line++
	return trimRecordNewline(rr.buf), rr.line, false, nil
}

// discard consumes the remainder of an over-sized record without buffering it
// and drops the buffer so a single huge record does not pin memory for the
// rest of the file.
func (rr *RecordReader) discard() ([]byte, int, bool, error) {
	for {
		_, readErr := rr.reader.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			continue
		}
		if readErr != nil && readErr != io.EOF {
			return nil, rr.line, false, readErr
		}
		rr.line++
		rr.buf = nil
		return nil, rr.line, true, nil
	}
}

func trimRecordNewline(raw []byte) []byte {
	for len(raw) > 0 && (raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r') {
		raw = raw[:len(raw)-1]
	}
	return raw
}
