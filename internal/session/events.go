package session

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Event struct {
	TS      time.Time      `json:"ts"`
	Type    string         `json:"type"`
	Content string         `json:"content,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type Sink struct {
	mu sync.Mutex
	f  *os.File
	w  *bufio.Writer
}

func NewSink(path string) (*Sink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Sink{f: f, w: bufio.NewWriterSize(f, 1<<20)}, nil
}

func (s *Sink) Add(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(e)
	_, _ = s.w.Write(append(b, '\n'))
	_ = s.w.Flush()
}

func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.w.Flush()
	return s.f.Close()
}

func ReadJSONL(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := []Event{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
