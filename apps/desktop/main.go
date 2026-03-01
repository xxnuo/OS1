package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"os1/core/orchestrator"
	"os1/core/platform"
)

type mockASR struct{}

func (m mockASR) Transcribe(ctx context.Context, sessionID string, segment []byte, meta orchestrator.SegmentMeta) (orchestrator.ASRResult, error) {
	text := strings.TrimSpace(string(segment))
	if text == "" {
		text = "萨曼莎，继续"
	}
	return orchestrator.ASRResult{Text: text, Confidence: 0.97, LatencyMs: 32}, nil
}

type mockLLM struct{}

func (m mockLLM) Respond(ctx context.Context, sessionID string, userText string) (orchestrator.LLMDecision, error) {
	reply := "收到，我现在处理：" + userText
	intent := "chat"
	actions := []string{}
	if strings.Contains(userText, "任务") || strings.Contains(userText, "安排") {
		intent = "task_create"
		actions = []string{"task.create"}
	}
	if strings.Contains(userText, "暂停") {
		intent = "task_pause"
		actions = []string{"task.pause"}
		reply = "已暂停当前操作，等待你下一步指令。"
	}
	return orchestrator.LLMDecision{Intent: intent, ReplyText: reply, Actions: actions}, nil
}

type mockTTS struct{}

func (m mockTTS) Synthesize(ctx context.Context, sessionID string, text string) (<-chan orchestrator.TTSChunk, error) {
	ch := make(chan orchestrator.TTSChunk, 16)
	go func() {
		defer close(ch)
		parts := strings.Fields(text)
		if len(parts) == 0 {
			parts = []string{text}
		}
		for _, part := range parts {
			select {
			case <-ctx.Done():
				return
			case <-time.After(90 * time.Millisecond):
				ch <- orchestrator.TTSChunk{Chunk: part, IsFinal: false}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(60 * time.Millisecond):
			ch <- orchestrator.TTSChunk{Chunk: "", IsFinal: true}
		}
	}()
	return ch, nil
}

type server struct {
	runtime        *orchestrator.Runtime
	bus            *orchestrator.Bus
	defaultSession string
	auditPath      string
}

type startSessionReq struct {
	SessionID string `json:"sessionId"`
}

type audioReq struct {
	SessionID  string `json:"sessionId"`
	Text       string `json:"text"`
	DurationMs int    `json:"durationMs"`
}

type muteReq struct {
	Enabled bool `json:"enabled"`
}

type interruptReq struct {
	SessionID string `json:"sessionId"`
}

type registerCapabilityReq struct {
	SessionID    string          `json:"sessionId"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	SideEffects  []string        `json:"sideEffects"`
	Source       string          `json:"source"`
	Signature    string          `json:"signature"`
}

type unregisterCapabilityReq struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
}

type signalWindowReq struct {
	App         string `json:"app"`
	WindowTitle string `json:"windowTitle"`
}

type signalInputReq struct {
	Active       bool   `json:"active"`
	LastActivity string `json:"lastActivity"`
}

type signalFileReq struct {
	Path      string `json:"path"`
	EventType string `json:"eventType"`
	Timestamp string `json:"timestamp"`
}

type signalClipboardReq struct {
	Timestamp string `json:"timestamp"`
}

type signalReq struct {
	SessionID string              `json:"sessionId"`
	Type      string              `json:"type"`
	Window    *signalWindowReq    `json:"window"`
	Input     *signalInputReq     `json:"input"`
	File      *signalFileReq      `json:"file"`
	Clipboard *signalClipboardReq `json:"clipboard"`
}

type memoryCreateReq struct {
	SessionID  string   `json:"sessionId"`
	Kind       string   `json:"kind"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"source"`
}

type memoryUpdateReq struct {
	SessionID  string    `json:"sessionId"`
	ID         string    `json:"id"`
	Content    *string   `json:"content"`
	Tags       *[]string `json:"tags"`
	Confidence *float64  `json:"confidence"`
	Source     *string   `json:"source"`
}

type memoryDeleteReq struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
}

type memoryLockReq struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Locked    bool   `json:"locked"`
}

func main() {
	bus := orchestrator.NewBus()
	auditPath := ".tmp/audit.jsonl"
	audit, err := orchestrator.NewAuditWriter(auditPath)
	if err != nil {
		panic(err)
	}
	defer audit.Close()
	memoryStore, err := orchestrator.NewMemoryStore(".tmp/memory.json", os.Getenv("OS1_MEMORY_KEY"))
	if err != nil {
		panic(err)
	}
	runtime := orchestrator.NewRuntime(mockASR{}, mockLLM{}, mockTTS{}, bus, audit, platform.NewPlatformAdapter(), memoryStore)
	s := &server{runtime: runtime, bus: bus, defaultSession: "demo", auditPath: auditPath}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/audit/recent", s.handleAuditRecent)
	mux.HandleFunc("/api/session/start", s.handleSessionStart)
	mux.HandleFunc("/api/audio", s.handleAudio)
	mux.HandleFunc("/api/interrupt", s.handleInterrupt)
	mux.HandleFunc("/api/mute", s.handleMute)
	mux.HandleFunc("/api/hud/toggle", s.handleHUDToggle)
	mux.HandleFunc("/api/capabilities", s.handleCapabilities)
	mux.HandleFunc("/api/capabilities/register", s.handleCapabilityRegister)
	mux.HandleFunc("/api/capabilities/unregister", s.handleCapabilityUnregister)
	mux.HandleFunc("/api/signals", s.handleSignals)
	mux.HandleFunc("/api/memory", s.handleMemory)
	mux.HandleFunc("/api/memory/create", s.handleMemoryCreate)
	mux.HandleFunc("/api/memory/update", s.handleMemoryUpdate)
	mux.HandleFunc("/api/memory/delete", s.handleMemoryDelete)
	mux.HandleFunc("/api/memory/lock", s.handleMemoryLock)
	port := readPort()
	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: withCORS(mux),
	}
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()
	log.Printf("OS1 backend listening on %s", httpServer.Addr)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func readPort() string {
	raw := strings.TrimSpace(os.Getenv("OS1_PORT"))
	if raw == "" {
		return "7070"
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 65535 {
		return "7070"
	}
	return raw
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *server) resolveSessionID(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return s.defaultSession
	}
	return in
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleAuditRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	limit := 120
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	events, err := readAuditRecent(s.auditPath, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	state := s.runtime.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"mute":             state.Mute,
		"hudVisible":       state.HUDVisible,
		"operationsPaused": state.OperationsPaused,
	})
}

func (s *server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req startSessionReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	if err := s.runtime.StartSession(sid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": sid, "started": true})
}

func (s *server) handleAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req audioReq
	if !decodeJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text required"})
		return
	}
	duration := req.DurationMs
	if duration <= 0 {
		duration = 500
	}
	sid := s.resolveSessionID(req.SessionID)
	err := s.runtime.HandleUserAudio(sid, []byte(text), orchestrator.SegmentMeta{SampleRate: 16000, DurationMs: duration})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "sessionId": sid})
}

func (s *server) handleMute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req muteReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.runtime.SetMute(req.Enabled); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mute": req.Enabled})
}

func (s *server) handleHUDToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	visible, err := s.runtime.ToggleHUD()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hudVisible": visible})
}

func (s *server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req interruptReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	if err := s.runtime.InterruptSpeaking(sid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": sid, "interrupted": true})
}

func (s *server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	capabilities := s.runtime.ListCapabilities()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":        len(capabilities),
		"capabilities": capabilities,
	})
}

func (s *server) handleCapabilityRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req registerCapabilityReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	capability, err := s.runtime.RegisterCapability(sid, orchestrator.CapabilitySpec{
		Name:         req.Name,
		Version:      req.Version,
		InputSchema:  req.InputSchema,
		OutputSchema: req.OutputSchema,
		SideEffects:  req.SideEffects,
		Source:       req.Source,
		Signature:    req.Signature,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registered": true,
		"sessionId":  sid,
		"capability": capability,
	})
}

func (s *server) handleCapabilityUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req unregisterCapabilityReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	if err := s.runtime.UnregisterCapability(sid, req.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"unregistered": true,
		"sessionId":    sid,
		"id":           req.ID,
	})
}

func (s *server) handleSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req signalReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	parsed, err := parseSignalReq(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.runtime.IngestUserSignal(sid, parsed); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"sessionId": sid,
		"type":      req.Type,
	})
}

func (s *server) handleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	includeDeleted := false
	raw := strings.TrimSpace(r.URL.Query().Get("includeDeleted"))
	if raw == "1" || strings.EqualFold(raw, "true") {
		includeDeleted = true
	}
	items := s.runtime.ListMemory(includeDeleted)
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(items),
		"items": items,
	})
}

func (s *server) handleMemoryCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req memoryCreateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "user"
	}
	item, err := s.runtime.CreateMemory(sid, orchestrator.MemoryCreate{
		Kind:       req.Kind,
		Content:    req.Content,
		Tags:       req.Tags,
		Confidence: req.Confidence,
		Source:     source,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created":   true,
		"sessionId": sid,
		"item":      item,
	})
}

func (s *server) handleMemoryUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req memoryUpdateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	item, err := s.runtime.UpdateMemory(sid, req.ID, orchestrator.MemoryPatch{
		Content:    req.Content,
		Tags:       req.Tags,
		Confidence: req.Confidence,
		Source:     req.Source,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated":   true,
		"sessionId": sid,
		"item":      item,
	})
}

func (s *server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req memoryDeleteReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	if err := s.runtime.DeleteMemory(sid, req.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   true,
		"sessionId": sid,
		"id":        req.ID,
	})
}

func (s *server) handleMemoryLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req memoryLockReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sid := s.resolveSessionID(req.SessionID)
	item, err := s.runtime.LockMemory(sid, req.ID, req.Locked)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"locked":    true,
		"sessionId": sid,
		"item":      item,
	})
}

func parseSignalReq(req signalReq) (orchestrator.UserSignal, error) {
	kind := orchestrator.UserSignalType(strings.TrimSpace(req.Type))
	switch kind {
	case orchestrator.WindowSignalType:
		if req.Window == nil {
			return orchestrator.UserSignal{}, errors.New("missing window payload")
		}
		return orchestrator.UserSignal{
			Type: kind,
			Window: &orchestrator.WindowSignal{
				App:         req.Window.App,
				WindowTitle: req.Window.WindowTitle,
			},
		}, nil
	case orchestrator.InputSignalType:
		if req.Input == nil {
			return orchestrator.UserSignal{}, errors.New("missing input payload")
		}
		lastActivity, err := parseSignalTime(req.Input.LastActivity)
		if err != nil {
			return orchestrator.UserSignal{}, err
		}
		return orchestrator.UserSignal{
			Type: kind,
			Input: &orchestrator.InputSignal{
				Active:       req.Input.Active,
				LastActivity: lastActivity,
			},
		}, nil
	case orchestrator.FileSignalType:
		if req.File == nil {
			return orchestrator.UserSignal{}, errors.New("missing file payload")
		}
		ts, err := parseSignalTime(req.File.Timestamp)
		if err != nil {
			return orchestrator.UserSignal{}, err
		}
		return orchestrator.UserSignal{
			Type: kind,
			File: &orchestrator.FileSignal{
				Path:      req.File.Path,
				EventType: req.File.EventType,
				Timestamp: ts,
			},
		}, nil
	case orchestrator.ClipboardSignalType:
		if req.Clipboard == nil {
			return orchestrator.UserSignal{}, errors.New("missing clipboard payload")
		}
		ts, err := parseSignalTime(req.Clipboard.Timestamp)
		if err != nil {
			return orchestrator.UserSignal{}, err
		}
		return orchestrator.UserSignal{
			Type: kind,
			Clipboard: &orchestrator.ClipboardSignal{
				Timestamp: ts,
			},
		}, nil
	default:
		return orchestrator.UserSignal{}, fmt.Errorf("unsupported signal type: %s", req.Type)
	}
}

func parseSignalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC(), nil
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, errors.New("invalid timestamp")
	}
	return ts.UTC(), nil
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "stream unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, ch, unsub := s.bus.Subscribe(512)
	defer unsub()
	state := s.runtime.Snapshot()
	initEvent := orchestrator.Event{
		Type: orchestrator.AuditEventType,
		Time: time.Now(),
		Payload: map[string]any{
			"mute":             state.Mute,
			"hudVisible":       state.HUDVisible,
			"operationsPaused": state.OperationsPaused,
		},
	}
	if err := streamEvent(w, initEvent); err != nil {
		return
	}
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if err := streamEvent(w, e); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func streamEvent(w http.ResponseWriter, e orchestrator.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", b)
	return err
}

func readAuditRecent(path string, limit int) ([]orchestrator.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	ring := make([]string, limit)
	count := 0
	for scanner.Scan() {
		ring[count%limit] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	start := 0
	if count > limit {
		start = count - limit
	}
	out := make([]orchestrator.Event, 0, min(count, limit))
	for i := start; i < count; i++ {
		line := ring[i%limit]
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e orchestrator.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
