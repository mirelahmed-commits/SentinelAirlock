package events

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Event struct {
	TS       time.Time      `json:"ts"`
	Type     string         `json:"type"`
	Path     string         `json:"path,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Diff     string         `json:"diff,omitempty"`
	Policy   map[string]any `json:"policy,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
	Risk     map[string]any `json:"risk,omitempty"`
	Approval map[string]any `json:"approval,omitempty"`
}

type Logger struct {
	mu     sync.Mutex
	f      *os.File
	w      *bufio.Writer
	events []Event
}

func NewLogger(jsonlPath string) (*Logger, error) {
	f, err := os.Create(jsonlPath)
	if err != nil {
		return nil, err
	}
	return &Logger{
		f: f,
		w: bufio.NewWriterSize(f, 1<<20),
	}, nil
}

func (l *Logger) Add(e Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, e)
	b, _ := json.Marshal(e)
	_, _ = l.w.Write(append(b, '\n'))
	_ = l.w.Flush()
}

func (l *Logger) EventsSnapshot() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.w.Flush()
	return l.f.Close()
}

func AppendJSONLEvent(jsonlPath string, e Event) error {
	f, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func ReadJSONL(jsonlPath string) ([]Event, error) {
	f, err := os.Open(jsonlPath)
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
	if err := sc.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	return out, nil
}
