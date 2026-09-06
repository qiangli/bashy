package harnessrunner

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sync"
)

type boundedSpool struct {
	mu      sync.Mutex
	file    *os.File
	limit   int64
	written int64
	omitted int64
}

func newBoundedSpool(path string, limit int64) (*boundedSpool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &boundedSpool{file: file, limit: limit}, nil
}

func (s *boundedSpool) Write(value []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	original := len(value)
	allowed := int64(len(value))
	if s.limit > 0 && s.written+allowed > s.limit {
		allowed = s.limit - s.written
		if allowed < 0 {
			allowed = 0
		}
	}
	if allowed > 0 {
		written, err := s.file.Write(value[:allowed])
		s.written += int64(written)
		if err != nil {
			return written, err
		}
		if int64(written) != allowed {
			return written, io.ErrShortWrite
		}
	}
	s.omitted += int64(original) - allowed
	// A bounded sink consumed the source bytes even when it deliberately did
	// not retain them. Returning a short write would incorrectly fail the job.
	return original, nil
}

func (s *boundedSpool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func (s *boundedSpool) stats() (written, omitted int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written, s.omitted
}

func readChunk(path string, cursor, maximum int64) ([]OutputChunk, int64, error) {
	if cursor < 0 {
		return nil, cursor, fmt.Errorf("cursor cannot be negative")
	}
	if maximum <= 0 {
		maximum = 64 << 10
	}
	if maximum > 1<<20 {
		maximum = 1 << 20
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, cursor, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, cursor, err
	}
	if cursor > info.Size() {
		return nil, cursor, fmt.Errorf("cursor %d exceeds output size %d", cursor, info.Size())
	}
	remaining := info.Size() - cursor
	if remaining == 0 {
		return nil, cursor, nil
	}
	if remaining > maximum {
		remaining = maximum
	}
	buf := make([]byte, remaining)
	n, err := file.ReadAt(buf, cursor)
	if err != nil && err != io.EOF {
		return nil, cursor, err
	}
	buf = buf[:n]
	next := cursor + int64(n)
	return []OutputChunk{{From: cursor, To: next, Encoding: "base64", Data: base64.StdEncoding.EncodeToString(buf)}}, next, nil
}
