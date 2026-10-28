// Package pi reads what pi leaves on disk for driver: the session JSONL
// (tailed from a saved byte offset, parsed for the operator's messages and
// for bash tool calls and their results), the status.json the driver
// extension writes, `pi --list-models` output and the default model in pi's
// settings. It never runs a pi agent.
package pi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// maxTailRead bounds how many bytes one ReadNewLines call consumes, so a
// multi-megabyte catch-up (a restart after a long session) is spread over
// several ticks instead of one.
const maxTailRead = 4 << 20

// ReadNewLines returns the complete lines appended to path since offset and
// the offset just past the last complete line. A partial trailing line (pi
// mid-write) is not returned and not consumed, so the next call re-reads it
// whole. A file shorter than offset (truncated or replaced) restarts at 0. A
// missing file is (nil, offset, nil).
func ReadNewLines(path string, offset int64) (lines [][]byte, next int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, offset, nil
		}
		return nil, offset, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, offset, fmt.Errorf("stat %s: %w", path, err)
	}
	if offset > st.Size() || offset < 0 {
		offset = 0
	}
	if offset == st.Size() {
		return nil, offset, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek %s: %w", path, err)
	}
	buf, err := io.ReadAll(io.LimitReader(f, maxTailRead))
	if err != nil {
		return nil, offset, fmt.Errorf("read %s: %w", path, err)
	}
	consumed := 0
	for {
		i := bytes.IndexByte(buf[consumed:], '\n')
		if i < 0 {
			break
		}
		line := buf[consumed : consumed+i]
		consumed += i + 1
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines, offset + int64(consumed), nil
}
