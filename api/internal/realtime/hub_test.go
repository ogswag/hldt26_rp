package realtime

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

func presenceList(t *testing.T, h *Hub, project uuid.UUID) []Presence {
	t.Helper()
	var v struct {
		List []Presence `json:"list"`
	}
	if err := json.Unmarshal(h.Presence(project), &v); err != nil {
		t.Fatal(err)
	}
	return v.List
}

func report(h *Hub, project uuid.UUID, user, email, client, route string) {
	h.updatePresence(project, presenceMsg{Presence: Presence{ClientID: client, UserID: user, Email: email, Route: route, Selection: json.RawMessage("null")}})
}

func TestPresenceKeepsUsersApart(t *testing.T) {
	h := New("", slog.New(slog.NewTextHandler(io.Discard, nil)))
	project := uuid.New()

	report(h, project, "u1", "anna@example.com", "tab", "/p/x/object")
	report(h, project, "u2", "maks@example.com", "tab", "/p/x/robots")
	if got := presenceList(t, h, project); len(got) != 2 {
		t.Fatalf("same client id from two users must give two entries, got %+v", got)
	}

	h.updatePresence(project, presenceMsg{Leave: true, Presence: Presence{ClientID: "tab", UserID: "u2"}})
	got := presenceList(t, h, project)
	if len(got) != 1 || got[0].UserID != "u1" || got[0].Route != "/p/x/object" {
		t.Fatalf("a leave must remove only the caller's entry, got %+v", got)
	}
}

func TestPresenceOneUserManyPages(t *testing.T) {
	h := New("", slog.New(slog.NewTextHandler(io.Discard, nil)))
	project := uuid.New()

	report(h, project, "u1", "anna@example.com", "page-a", "/p/x/object")
	report(h, project, "u1", "anna@example.com", "page-b", "/p/x/calc")
	report(h, project, "u1", "anna@example.com", "page-a", "/p/x/robots")
	got := presenceList(t, h, project)
	if len(got) != 2 {
		t.Fatalf("one user with two pages must give two entries, got %+v", got)
	}
	routes := map[string]string{}
	for _, e := range got {
		routes[e.ClientID] = e.Route
	}
	if routes["page-a"] != "/p/x/robots" || routes["page-b"] != "/p/x/calc" {
		t.Fatalf("routes %+v", routes)
	}
}
