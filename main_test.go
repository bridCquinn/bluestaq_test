package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resetStore clears the global in-memory state before each test run
// to ensure test isolation and prevent cross-contamination.
func resetStore() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.notes = make(map[string]Note)
	store.nextID = 1
}

func TestHandleCreateNote(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		expectedStatus int
	}{
		{
			name:           "Valid Note Creation",
			payload:        `{"team_id": "engineering", "title": "Architecture", "content": "Framework-free Go", "author": "Alice"}`,
			expectedStatus: http.StatusCreated, // 201
		},
		{
			name:           "Missing Invariant: Title",
			payload:        `{"team_id": "engineering", "content": "No title here", "author": "Alice"}`,
			expectedStatus: http.StatusUnprocessableEntity, // 422
		},
		{
			name:           "Missing Invariant: TeamID",
			payload:        `{"title": "Orphan Note", "content": "No team here", "author": "Alice"}`,
			expectedStatus: http.StatusUnprocessableEntity, // 422
		},
		{
			name:           "Malformed JSON Layout",
			payload:        `{"team_id": "engineering", "title": "Bad JSON", broken`,
			expectedStatus: http.StatusBadRequest, // 400
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetStore()

			req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handleCreateNote(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// For successful creation, ensure a valid ID was assigned
			if tt.expectedStatus == http.StatusCreated {
				var created Note
				if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if created.ID != "1" {
					t.Errorf("expected generated ID '1', got '%s'", created.ID)
				}
			}
		})
	}
}

func TestHandleGetNotes_MultiTenancy(t *testing.T) {
	resetStore()

	// Seed the memory store with notes across isolated team boundaries
	store.mu.Lock()
	store.notes["1"] = Note{ID: "1", TeamID: "engineering", Title: "Eng Note", Content: "Sensitive details"}
	store.notes["2"] = Note{ID: "2", TeamID: "marketing", Title: "Mkt Note", Content: "Public launch"}
	store.mu.Unlock()

	// Request notes ONLY for the engineering team boundary
	req := httptest.NewRequest(http.MethodGet, "/notes?team_id=engineering", nil)
	rr := httptest.NewRecorder()

	handleGetNotes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var results []Note
	if err := json.Unmarshal(rr.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	// Assert that multi-tenancy isolation filtered out the marketing note
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 note, got %d", len(results))
	}

	if results[0].TeamID != "engineering" {
		t.Errorf("data leak: received note belonging to team '%s'", results[0].TeamID)
	}
}

func TestHandleGetNotes_MissingBoundary(t *testing.T) {
	resetStore()

	// Send a request missing the mandatory team_id filter query
	req := httptest.NewRequest(http.MethodGet, "/notes", nil)
	rr := httptest.NewRecorder()

	handleGetNotes(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 due to missing boundary constraint, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "team_id required") {
		t.Errorf("expected descriptive error string, got: %s", rr.Body.String())
	}
}
