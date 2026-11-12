package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTP_PinnedHostIgnoresClientHost(t *testing.T) {
	umami := setupTestUmamiServer()
	defer umami.Close()

	handler := NewHTTPHandler(nil, 0)
	handler.SetPinnedHost(umami.URL)

	// Client points at a host that doesn't exist; the pinned host must win.
	sessionID := initializeSession(t, handler, "http://attacker.invalid")
	if sessionID == "" {
		t.Fatal("expected a session against the pinned host")
	}
}

func TestHTTP_PinnedHostNoClientHostNeeded(t *testing.T) {
	umami := setupTestUmamiServer()
	defer umami.Close()

	handler := NewHTTPHandler(nil, 0)
	handler.SetPinnedHost(umami.URL)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("X-Umami-Username", "admin")
	req.Header.Set("X-Umami-Password", "pass")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Mcp-Session-Id") == "" {
		t.Fatalf("expected session without X-Umami-Host, got: %s", w.Body.String())
	}
}

func TestHTTP_ReapIdleSessions(t *testing.T) {
	umami := setupTestUmamiServer()
	defer umami.Close()

	handler := NewHTTPHandler(nil, 2)
	stale := initializeSession(t, handler, umami.URL)
	fresh := initializeSession(t, handler, umami.URL)

	// Backdate one session past the TTL.
	val, _ := handler.sessions.Load(stale)
	val.(*session).lastUsed.Store(time.Now().Add(-2 * time.Hour).UnixNano())

	if n := handler.reapIdle(time.Hour); n != 1 {
		t.Fatalf("expected 1 reaped session, got %d", n)
	}
	if _, ok := handler.sessions.Load(stale); ok {
		t.Error("stale session still present")
	}
	if _, ok := handler.sessions.Load(fresh); !ok {
		t.Error("fresh session was reaped")
	}
	if got := handler.sessionCount.Load(); got != 1 {
		t.Errorf("sessionCount = %d, want 1", got)
	}

	// Freed slot means a new session fits under MAX_SESSIONS=2 again.
	_ = initializeSession(t, handler, umami.URL)
}
