package main

import (
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

func main() {
	bus := orchestrator.NewBus()
	audit, err := orchestrator.NewAuditWriter(".tmp/audit.jsonl")
	if err != nil {
		panic(err)
	}
	defer audit.Close()
	runtime := orchestrator.NewRuntime(mockASR{}, mockLLM{}, mockTTS{}, bus, audit, platform.NewPlatformAdapter())
	s := &server{runtime: runtime, bus: bus, defaultSession: "demo"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/session/start", s.handleSessionStart)
	mux.HandleFunc("/api/audio", s.handleAudio)
	mux.HandleFunc("/api/interrupt", s.handleInterrupt)
	mux.HandleFunc("/api/mute", s.handleMute)
	mux.HandleFunc("/api/hud/toggle", s.handleHUDToggle)
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

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	state := s.runtime.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"mute":       state.Mute,
		"hudVisible": state.HUDVisible,
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
			"mute":        state.Mute,
			"hud_visible": state.HUDVisible,
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
