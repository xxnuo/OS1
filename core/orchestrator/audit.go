package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AuditWriter struct {
	mu   sync.Mutex
	file *os.File
}

func NewAuditWriter(path string) (*AuditWriter, error) {
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &AuditWriter{file: f}, nil
}

func (a *AuditWriter) Write(event Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = a.file.Write(append(b, '\n'))
	return err
}

func (a *AuditWriter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file == nil {
		return nil
	}
	err := a.file.Close()
	a.file = nil
	return err
}
