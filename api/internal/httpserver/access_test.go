package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"moscow_hackathon_2026/api/internal/config"
)

func TestProjectRoutesAreScoped(t *testing.T) {
	s, r := newServer(config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil)
	seen := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		seen[key] = true
		if !strings.Contains(route, "{id}") && !strings.Contains(route, "{runId}") && !strings.Contains(route, "{jobId}") {
			return nil
		}
		if _, ok := s.scopedRoutes[key]; !ok {
			t.Errorf("%s is not registered through scoped", key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key := range s.scopedRoutes {
		if !seen[key] {
			t.Errorf("scoped route %s is missing from the router", key)
		}
	}
}

func TestRoleAllows(t *testing.T) {
	for _, c := range []struct {
		role string
		need accessLevel
		want bool
	}{
		{RoleOwner, accessOwner, true},
		{RoleOwner, accessRead, true},
		{RoleEditor, accessWrite, true},
		{RoleEditor, accessOwner, false},
		{RoleViewer, accessRead, true},
		{RoleViewer, accessWrite, false},
		{"", accessRead, false},
	} {
		if got := roleAllows(c.role, c.need); got != c.want {
			t.Errorf("roleAllows(%q, %d) = %v", c.role, c.need, got)
		}
	}
}
