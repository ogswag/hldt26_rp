package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/realtime"
)

type sseEvent struct {
	name string
	data string
}

type sseStream struct {
	events chan sseEvent
	status int
}

// openStream connects c to the project's events and parses them in the background. The channel closes when the
// server ends the stream.
func openStream(t *testing.T, base string, c *itClient, pid string) *sseStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/projects/"+pid+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &sseStream{events: make(chan sseEvent, 64), status: resp.StatusCode}
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	if resp.StatusCode != http.StatusOK {
		close(s.events)
		return s
	}
	go func() {
		defer close(s.events)
		sc := bufio.NewScanner(resp.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.name != "" {
					s.events <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, ":"):
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return s
}

// next returns the next event named want, or fails after 5 s. Presence and runs are skipped, and so is ops when
// waiting for something else: a calculation may journal its selection.
func (s *sseStream) next(t *testing.T, want string) sseEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				t.Fatalf("stream closed while waiting for %s", want)
			}
			if ev.name == want {
				return ev
			}
			if ev.name != realtime.EventPresence && ev.name != realtime.EventRuns && ev.name != realtime.EventOps {
				t.Fatalf("got %s %s while waiting for %s", ev.name, ev.data, want)
			}
		case <-deadline:
			t.Fatalf("no %s event", want)
		}
	}
}

// presenceUntil reads presence events until the list satisfies ok, and returns it.
func presenceUntil(t *testing.T, s *sseStream, ok func([]realtime.Presence) bool) []realtime.Presence {
	t.Helper()
	for {
		var v struct {
			List []realtime.Presence `json:"list"`
		}
		if err := json.Unmarshal([]byte(s.next(t, realtime.EventPresence).data), &v); err != nil {
			t.Fatal(err)
		}
		if ok(v.List) {
			return v.List
		}
	}
}

// closed waits for the server to end the stream.
func (s *sseStream) closed(t *testing.T) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-s.events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream still open")
		}
	}
}

func seqOf(t *testing.T, ev sseEvent) int64 {
	t.Helper()
	var v struct {
		Seq int64 `json:"seq"`
	}
	if err := json.Unmarshal([]byte(ev.data), &v); err != nil {
		t.Fatalf("ops data %q: %v", ev.data, err)
	}
	return v.Seq
}

func TestProjectEvents(t *testing.T) {
	env := newEnv(t)
	env.srv.heartbeat = 100 * time.Millisecond
	ts := httptest.NewServer(env.h)
	t.Cleanup(ts.Close)
	select {
	case <-env.hub.Ready():
	case <-time.After(10 * time.Second):
		t.Fatal("hub is not listening")
	}
	ctx := context.Background()
	owner := register(t, env.h, "sse-owner@example.com")
	viewer := register(t, env.h, "sse-viewer@example.com")
	p, _ := warehouseProject(t, owner, "Живой проект")
	addMember(t, owner, p.ID, "sse-viewer@example.com", RoleViewer)

	vs := openStream(t, ts.URL, viewer, p.ID)
	ownerStream := openStream(t, ts.URL, owner, p.ID)
	start := seqOf(t, vs.next(t, realtime.EventOps))
	if start != draftSeq(t, env, p.ID) {
		t.Fatalf("first seq %d", start)
	}
	if ev := vs.next(t, realtime.EventPresence); ev.data != `{"list":[]}` {
		t.Fatalf("presence %s", ev.data)
	}
	ownerStream.next(t, realtime.EventOps)

	// An edit reaches both streams.
	postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "name", "Живой проект 2")))
	if got := seqOf(t, vs.next(t, realtime.EventOps)); got != start+1 {
		t.Fatalf("viewer seq %d", got)
	}
	ownerStream.next(t, realtime.EventOps)

	// Presence: joining and leaving.
	variant := getFull(t, owner, p.ID).Variants[0].ID
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{
		"client_id": "tab-1", "route": "/variants", "selection": map[string]string{"coll": "variants", "id": variant},
	}, http.StatusNoContent, nil)
	var pres struct {
		List []realtime.Presence `json:"list"`
	}
	if err := json.Unmarshal([]byte(vs.next(t, realtime.EventPresence).data), &pres); err != nil || len(pres.List) != 1 ||
		pres.List[0].Email != "sse-owner@example.com" || pres.List[0].Route != "/variants" || !strings.Contains(string(pres.List[0].Selection), variant) {
		t.Fatalf("presence %+v", pres)
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-1", "leave": true}, http.StatusNoContent, nil)
	if ev := vs.next(t, realtime.EventPresence); ev.data != `{"list":[]}` {
		t.Fatalf("after leave %s", ev.data)
	}
	viewer.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-2", "route": "/map", "selection": nil}, http.StatusNoContent, nil)
	ownerStream.next(t, realtime.EventPresence)
	viewer.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-2", "selection": map[string]string{"coll": "x"}, "extra": 1}, http.StatusBadRequest, nil)

	// A client id another member already uses gets its own entry; a leave touches only the caller's.
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-2", "route": "/robots"}, http.StatusNoContent, nil)
	presenceUntil(t, vs, func(l []realtime.Presence) bool { return len(l) == 2 })
	viewer.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-2", "leave": true}, http.StatusNoContent, nil)
	left := presenceUntil(t, vs, func(l []realtime.Presence) bool { return len(l) == 1 })
	if left[0].Email != "sse-owner@example.com" || left[0].Route != "/robots" {
		t.Fatalf("after the viewer's leave %+v", left)
	}
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/presence", map[string]any{"client_id": "tab-2", "leave": true}, http.StatusNoContent, nil)
	presenceUntil(t, vs, func(l []realtime.Presence) bool { return len(l) == 0 })

	// A calculation announces runs.
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", nil, http.StatusOK, nil)
	deadline := time.After(5 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-vs.events:
			got = ev.name == realtime.EventRuns
		case <-deadline:
			t.Fatal("no runs event")
		}
	}

	// A role change reaches the member with the new role.
	var members struct {
		Items []memberJSON `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/members", nil, http.StatusOK, &members)
	viewerID := members.Items[1].UserID
	owner.json(http.MethodPut, "/api/projects/"+p.ID+"/members/"+viewerID, map[string]any{"role": RoleEditor}, http.StatusNoContent, nil)
	if ev := vs.next(t, realtime.EventAccess); ev.data != `{"role":"editor"}` {
		t.Fatalf("access %s", ev.data)
	}

	// A lost listener connection resets every stream.
	if _, err := env.db.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = 'realtime-hub' AND datname = current_database()`); err != nil {
		t.Fatal(err)
	}
	vs.next(t, realtime.EventReset)
	ownerStream.next(t, realtime.EventReset)
	postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "name", "После переподключения")))
	vs.next(t, realtime.EventOps)

	// Per-user stream limit.
	env.hub.SetMaxPerUser(2)
	openStream(t, ts.URL, viewer, p.ID).next(t, realtime.EventOps)
	if extra := openStream(t, ts.URL, viewer, p.ID); extra.status != http.StatusTooManyRequests {
		t.Fatalf("third stream: %d", extra.status)
	}

	// Removing the member ends their stream with a null role.
	owner.json(http.MethodDelete, "/api/projects/"+p.ID+"/members/"+viewerID, nil, http.StatusNoContent, nil)
	if ev := vs.next(t, realtime.EventAccess); ev.data != `{"role":null}` {
		t.Fatalf("removed %s", ev.data)
	}
	vs.closed(t)
	if s := openStream(t, ts.URL, viewer, p.ID); s.status != http.StatusNotFound {
		t.Fatalf("removed member reconnects: %d", s.status)
	}

	// Deleting the project ends every stream.
	other := openStream(t, ts.URL, owner, p.ID)
	other.next(t, realtime.EventOps)
	owner.json(http.MethodDelete, "/api/projects/"+p.ID, nil, http.StatusOK, nil)
	ownerStream.next(t, realtime.EventDeleted)
	ownerStream.closed(t)
	other.closed(t)

	// Logging out ends the session's streams at the next heartbeat.
	q, _ := warehouseProject(t, owner, "Второй")
	last := openStream(t, ts.URL, owner, q.ID)
	last.next(t, realtime.EventOps)
	owner.json(http.MethodPost, "/api/auth/logout", nil, http.StatusNoContent, nil)
	last.closed(t)

	// Streams are read-only and need an account.
	guest := &itClient{t: t, h: env.h}
	if s := openStream(t, ts.URL, guest, uuid.NewString()); s.status != http.StatusForbidden {
		t.Fatalf("guest stream: %d", s.status)
	}
}
