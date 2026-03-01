package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type UserSignalType string

const (
	WindowSignalType    UserSignalType = "window"
	InputSignalType     UserSignalType = "input"
	FileSignalType      UserSignalType = "file"
	ClipboardSignalType UserSignalType = "clipboard"
)

type WindowSignal struct {
	App         string `json:"app"`
	WindowTitle string `json:"windowTitle,omitempty"`
}

type InputSignal struct {
	Active       bool      `json:"active"`
	LastActivity time.Time `json:"lastActivity"`
}

type FileSignal struct {
	Path      string    `json:"path"`
	EventType string    `json:"eventType"`
	Timestamp time.Time `json:"timestamp"`
}

type ClipboardSignal struct {
	Timestamp time.Time `json:"timestamp"`
}

type UserSignal struct {
	Type      UserSignalType   `json:"type"`
	Window    *WindowSignal    `json:"window,omitempty"`
	Input     *InputSignal     `json:"input,omitempty"`
	File      *FileSignal      `json:"file,omitempty"`
	Clipboard *ClipboardSignal `json:"clipboard,omitempty"`
}

type UserSignalBatch struct {
	SignalCount      int           `json:"signalCount"`
	From             time.Time     `json:"from"`
	To               time.Time     `json:"to"`
	Window           *WindowSignal `json:"window,omitempty"`
	Input            *InputSignal  `json:"input,omitempty"`
	FileChanges      []FileSignal  `json:"fileChanges,omitempty"`
	ClipboardChanged bool          `json:"clipboardChanged"`
}

type signalBuffer struct {
	from             time.Time
	to               time.Time
	signalCount      int
	window           *WindowSignal
	input            *InputSignal
	fileChanges      []FileSignal
	clipboardChanged bool
	timer            *time.Timer
}

func (r *Runtime) IngestUserSignal(sessionID string, signal UserSignal) error {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	now := time.Now().UTC()
	switch signal.Type {
	case WindowSignalType:
		if signal.Window == nil {
			return errors.New("missing window payload")
		}
		payload := WindowSignal{
			App:         strings.TrimSpace(signal.Window.App),
			WindowTitle: strings.TrimSpace(signal.Window.WindowTitle),
		}
		if payload.App == "" {
			return errors.New("empty window app")
		}
		r.emit(Event{Type: WindowChangedEventType, SessionID: sid, Time: now, Payload: payload})
		r.enqueueSignal(sid, now, func(buf *signalBuffer) {
			buf.window = &payload
		})
	case InputSignalType:
		if signal.Input == nil {
			return errors.New("missing input payload")
		}
		last := signal.Input.LastActivity
		if last.IsZero() {
			last = now
		}
		payload := InputSignal{
			Active:       signal.Input.Active,
			LastActivity: last.UTC(),
		}
		r.emit(Event{Type: InputActivityEventType, SessionID: sid, Time: now, Payload: payload})
		r.enqueueSignal(sid, now, func(buf *signalBuffer) {
			buf.input = &payload
		})
	case FileSignalType:
		if signal.File == nil {
			return errors.New("missing file payload")
		}
		path := strings.TrimSpace(signal.File.Path)
		if path == "" {
			return errors.New("empty file path")
		}
		eventType := strings.TrimSpace(signal.File.EventType)
		if eventType == "" {
			return errors.New("empty file event type")
		}
		ts := signal.File.Timestamp
		if ts.IsZero() {
			ts = now
		}
		payload := FileSignal{
			Path:      path,
			EventType: eventType,
			Timestamp: ts.UTC(),
		}
		r.emit(Event{Type: FileChangedEventType, SessionID: sid, Time: now, Payload: payload})
		r.enqueueSignal(sid, now, func(buf *signalBuffer) {
			buf.fileChanges = append(buf.fileChanges, payload)
		})
	case ClipboardSignalType:
		if signal.Clipboard == nil {
			return errors.New("missing clipboard payload")
		}
		ts := signal.Clipboard.Timestamp
		if ts.IsZero() {
			ts = now
		}
		payload := ClipboardSignal{Timestamp: ts.UTC()}
		r.emit(Event{Type: ClipboardChangedEventType, SessionID: sid, Time: now, Payload: payload})
		r.enqueueSignal(sid, now, func(buf *signalBuffer) {
			buf.clipboardChanged = true
		})
	default:
		return fmt.Errorf("unsupported signal type: %s", signal.Type)
	}
	return nil
}

func (r *Runtime) enqueueSignal(sessionID string, now time.Time, apply func(*signalBuffer)) {
	r.signalMu.Lock()
	buf, ok := r.signalBuffers[sessionID]
	if !ok {
		buf = &signalBuffer{}
		r.signalBuffers[sessionID] = buf
	}
	if buf.from.IsZero() {
		buf.from = now
	}
	buf.to = now
	buf.signalCount++
	apply(buf)
	if buf.timer != nil {
		buf.timer.Stop()
	}
	current := buf
	delay := r.signalWindow
	buf.timer = time.AfterFunc(delay, func() {
		r.flushSignalBuffer(sessionID, current)
	})
	r.signalMu.Unlock()
}

func (r *Runtime) flushSignalBuffer(sessionID string, target *signalBuffer) {
	r.signalMu.Lock()
	current, ok := r.signalBuffers[sessionID]
	if !ok || current != target {
		r.signalMu.Unlock()
		return
	}
	delete(r.signalBuffers, sessionID)
	r.signalMu.Unlock()
	payload := UserSignalBatch{
		SignalCount:      current.signalCount,
		From:             current.from,
		To:               current.to,
		Window:           cloneWindowSignal(current.window),
		Input:            cloneInputSignal(current.input),
		FileChanges:      cloneFileSignals(current.fileChanges),
		ClipboardChanged: current.clipboardChanged,
	}
	r.emit(Event{Type: UserSignalBatchEventType, SessionID: sessionID, Time: time.Now().UTC(), Payload: payload})
}

func cloneWindowSignal(input *WindowSignal) *WindowSignal {
	if input == nil {
		return nil
	}
	out := *input
	return &out
}

func cloneInputSignal(input *InputSignal) *InputSignal {
	if input == nil {
		return nil
	}
	out := *input
	return &out
}

func cloneFileSignals(input []FileSignal) []FileSignal {
	if len(input) == 0 {
		return nil
	}
	out := make([]FileSignal, len(input))
	copy(out, input)
	return out
}
