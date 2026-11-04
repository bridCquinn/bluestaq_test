package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Note represents the structural model of a shared note entity.
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	TeamID    string    `json:"team_id"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MemoryStore provides a concurrent-safe thread-isolated memory map.
type MemoryStore struct {
	mu     sync.RWMutex
	notes  map[string]Note
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		notes:  make(map[string]Note),
		nextID: 1,
	}
}

// Global engine instance
var store = NewMemoryStore()

// responseWriterInterceptor intercepts writing metrics to capture the raw HTTP status code
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (rwi *responseWriterInterceptor) WriteHeader(statusCode int) {
	rwi.statusCode = statusCode
	rwi.ResponseWriter.WriteHeader(statusCode)
}

// 🛠️ OBSERVABILITY UPGRADE: Centralized HTTP Request JSON Logger Middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Intercept the default writer to cleanly capture outgoing HTTP response codes
		interceptor := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(interceptor, r)

		// Emit an structured log entry for cloud log collectors to effortlessly parse
		slog.Info("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", interceptor.statusCode),
			slog.Duration("latency", time.Since(start)),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}

func main() {
	// 🛠️ OBSERVABILITY UPGRADE: Initialize system-wide standard JSON log router targeting stdout
	jsonLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(jsonLogger)

	mux := http.NewServeMux()

	// Leverage Go 1.22+ structural pattern routing matchers
	mux.HandleFunc("GET /notes", handleGetNotes)
	mux.HandleFunc("POST /notes", handleCreateNote)
	mux.HandleFunc("GET /notes/{id}", handleGetNoteByID)
	mux.HandleFunc("PUT /notes/{id}", handleUpdateNote)
	mux.HandleFunc("DELETE /notes/{id}", handleDeleteNote)

	// Wrap entire router inside our telemetry middleware intercept chain
	loggingChain := loggingMiddleware(mux)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      loggingChain,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	shutdownCtx, stopSignalCapture := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignalCapture()

	go func() {
		slog.Info("Core REST Engine initializing", slog.String("port", ":8080"))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Critical system breakdown encountered", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-shutdownCtx.Done()
	slog.Warn("Termination signal intercepted! Initiating graceful shutdown routine...")

	graceCtx, cancelGraceCtx := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelGraceCtx()

	if err := server.Shutdown(graceCtx); err != nil {
		slog.Error("Server forced to shutdown with active connections un-cleared", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("Server cleanly exited. All connections resolved.")
}

// handleCreateNote treats raw insertion with system timestamps
func handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		TeamID  string `json:"team_id"`
		Author  string `json:"author"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Malformed payload structural layout", http.StatusBadRequest)
		return
	}

	if payload.Title == "" || payload.TeamID == "" {
		http.Error(w, "Validation failed: title and team_id are invariant constraints", http.StatusUnprocessableEntity)
		return
	}

	store.mu.Lock()
	idStr := strconv.Itoa(store.nextID)
	store.nextID++

	now := time.Now()
	newNote := Note{
		ID:        idStr,
		Title:     payload.Title,
		Content:   payload.Content,
		TeamID:    payload.TeamID,
		Author:    payload.Author,
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.notes[idStr] = newNote
	store.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newNote)
}

// handleGetNotes isolates collections filtered explicitly by team boundaries
func handleGetNotes(w http.ResponseWriter, r *http.Request) {
	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		http.Error(w, "Query parameter boundary violation: team_id required", http.StatusBadRequest)
		return
	}

	store.mu.RLock()
	results := make([]Note, 0)
	for _, note := range store.notes {
		if note.TeamID == teamID {
			results = append(results, note)
		}
	}
	store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleGetNoteByID resolves pointer mapping for a single item
func handleGetNoteByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	store.mu.RLock()
	note, exists := store.notes[id]
	store.mu.RUnlock()

	if !exists {
		http.Error(w, "Requested Note context not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(note)
}

// handleUpdateNote overwrites contents while holding creation context invariant
func handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var payload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Malformed payload structural layout", http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	note, exists := store.notes[id]
	if !exists {
		store.mu.Unlock()
		http.Error(w, "Requested Note context not found", http.StatusNotFound)
		return
	}

	if payload.Title != "" {
		note.Title = payload.Title
	}
	note.Content = payload.Content
	note.UpdatedAt = time.Now()

	store.notes[id] = note
	store.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(note)
}

// handleDeleteNote frees keys safely from state maps
func handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	store.mu.Lock()
	_, exists := store.notes[id]
	if !exists {
		store.mu.Unlock()
		http.Error(w, "Requested Note context not found", http.StatusNotFound)
		return
	}
	delete(store.notes, id)
	store.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}
