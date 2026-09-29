// Package realtime fans Postgres notifications out to live project streams (SSE). Each API process keeps one
// dedicated LISTEN connection, so any number of instances see the same events.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Channels the hub listens on. project_ops carries "<project_id>:<seq>", the others JSON.
const (
	ChannelOps      = "project_ops"
	ChannelEvents   = "project_events"
	ChannelPresence = "project_presence"
)

// Event types sent to streams.
const (
	EventOps      = "ops"
	EventPresence = "presence"
	EventRuns     = "runs"
	EventAccess   = "access"
	EventDeleted  = "deleted"
	// EventReset follows a lost listener connection: notifications may be missing, so clients re-read.
	EventReset = "reset"
)

const (
	subBuffer         = 64
	defaultMaxPerUser = 20
	presenceTTL       = 90 * time.Second
	presenceThrottle  = 250 * time.Millisecond
	sweepEvery        = 5 * time.Second
	appName           = "realtime-hub"
)

var (
	ErrTooManyStreams = errors.New("realtime: too many streams")
	ErrClosed         = errors.New("realtime: hub closed")
)

// Event is one message for a stream. User is set for access events: the member whose role changed.
type Event struct {
	Type string
	Data json.RawMessage
	User uuid.UUID
}

// Sub is one open stream. Read C until Done closes: the hub drops a stream that falls behind or shuts down.
type Sub struct {
	Project uuid.UUID
	User    uuid.UUID
	C       chan Event
	done    chan struct{}
	once    sync.Once
	hub     *Hub
}

func (s *Sub) Done() <-chan struct{} { return s.done }

// Close unsubscribes. Safe to call more than once.
func (s *Sub) Close() { s.hub.remove(s) }

// Presence is one open tab in a project.
type Presence struct {
	ClientID  string          `json:"client_id"`
	UserID    string          `json:"user_id"`
	Email     string          `json:"email"`
	Route     string          `json:"route"`
	Selection json.RawMessage `json:"selection"`
	expires   time.Time
}

type presenceMsg struct {
	ProjectID string `json:"project_id"`
	Leave     bool   `json:"leave"`
	Presence
}

type projectState struct {
	subs     map[*Sub]struct{}
	presence map[string]Presence
	// pending marks a presence change waiting for the throttle window.
	pending bool
	lastOut time.Time
}

type Hub struct {
	dsn string
	log *slog.Logger

	mu         sync.Mutex
	maxPerUser int
	projects   map[uuid.UUID]*projectState
	perUser    map[uuid.UUID]int
	closed     bool

	ready     chan struct{}
	readyOnce sync.Once
}

func New(dsn string, log *slog.Logger) *Hub {
	return &Hub{
		dsn: dsn, log: log, maxPerUser: defaultMaxPerUser, projects: map[uuid.UUID]*projectState{}, perUser: map[uuid.UUID]int{},
		ready: make(chan struct{}),
	}
}

// SetMaxPerUser limits open streams per user across projects (20 by default).
func (h *Hub) SetMaxPerUser(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxPerUser = n
}

// Ready closes once the first LISTEN is in place.
func (h *Hub) Ready() <-chan struct{} { return h.ready }

func (h *Hub) project(id uuid.UUID) *projectState {
	p := h.projects[id]
	if p == nil {
		p = &projectState{subs: map[*Sub]struct{}{}, presence: map[string]Presence{}}
		h.projects[id] = p
	}
	return p
}

// Subscribe opens a stream for a user in a project.
func (h *Hub) Subscribe(project, user uuid.UUID) (*Sub, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	if h.perUser[user] >= h.maxPerUser {
		return nil, ErrTooManyStreams
	}
	s := &Sub{Project: project, User: user, C: make(chan Event, subBuffer), done: make(chan struct{}), hub: h}
	h.project(project).subs[s] = struct{}{}
	h.perUser[user]++
	return s, nil
}

func (h *Hub) remove(s *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(s)
}

func (h *Hub) dropLocked(s *Sub) {
	p := h.projects[s.Project]
	if p == nil {
		return
	}
	if _, ok := p.subs[s]; !ok {
		return
	}
	delete(p.subs, s)
	s.once.Do(func() { close(s.done) })
	if h.perUser[s.User]--; h.perUser[s.User] <= 0 {
		delete(h.perUser, s.User)
	}
	if len(p.subs) == 0 && len(p.presence) == 0 {
		delete(h.projects, s.Project)
	}
}

// Presence lists the project's open tabs, ordered by user and tab.
func (h *Hub) Presence(project uuid.UUID) json.RawMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	return presenceJSON(h.projects[project])
}

func presenceJSON(p *projectState) json.RawMessage {
	list := []Presence{}
	if p != nil {
		for _, e := range p.presence {
			list = append(list, e)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Email != list[j].Email {
			return list[i].Email < list[j].Email
		}
		return list[i].ClientID < list[j].ClientID
	})
	raw, _ := json.Marshal(map[string]any{"list": list})
	return raw
}

// sendLocked delivers without blocking. A stream whose buffer is full is dropped; its client reconnects and
// catches up from its seq.
func (h *Hub) sendLocked(p *projectState, ev Event) {
	for s := range p.subs {
		select {
		case s.C <- ev:
		default:
			h.log.Warn("realtime.drop", "op", "realtime.drop", "project_id", s.Project.String())
			h.dropLocked(s)
		}
	}
}

func (h *Hub) broadcast(project uuid.UUID, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.projects[project]; p != nil {
		h.sendLocked(p, ev)
	}
}

// CloseAll ends every stream, for server shutdown.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, p := range h.projects {
		for s := range p.subs {
			h.dropLocked(s)
		}
	}
}

// Run listens until ctx ends, reconnecting with backoff. After a reconnect every stream gets a reset.
func (h *Hub) Run(ctx context.Context) {
	go h.sweep(ctx)
	backoff := time.Second
	connected := false
	for ctx.Err() == nil {
		listened, err := h.listen(ctx, connected)
		if ctx.Err() != nil {
			return
		}
		connected = true
		if listened {
			backoff = time.Second
		}
		h.log.Error("realtime.listen", "op", "realtime.listen", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// listen reports whether LISTEN was in place before the connection failed.
func (h *Hub) listen(ctx context.Context, reconnect bool) (bool, error) {
	cfg, err := pgx.ParseConfig(h.dsn)
	if err != nil {
		return false, err
	}
	cfg.RuntimeParams["application_name"] = appName
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return false, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	for _, ch := range []string{ChannelOps, ChannelEvents, ChannelPresence} {
		if _, err := conn.Exec(ctx, "LISTEN "+ch); err != nil {
			return false, err
		}
	}
	h.readyOnce.Do(func() { close(h.ready) })
	if reconnect {
		h.resetAll()
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, err
		}
		h.dispatch(n.Channel, n.Payload)
	}
}

func (h *Hub) resetAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.projects {
		h.sendLocked(p, Event{Type: EventReset, Data: json.RawMessage(`{}`)})
	}
}

func (h *Hub) dispatch(channel, payload string) {
	switch channel {
	case ChannelOps:
		pid, seqText, ok := strings.Cut(payload, ":")
		project, err := uuid.Parse(pid)
		seq, serr := strconv.ParseInt(seqText, 10, 64)
		if !ok || err != nil || serr != nil {
			return
		}
		h.broadcast(project, Event{Type: EventOps, Data: json.RawMessage(`{"seq":` + strconv.FormatInt(seq, 10) + `}`)})
	case ChannelEvents:
		var m struct {
			ProjectID uuid.UUID  `json:"project_id"`
			Type      string     `json:"type"`
			UserID    *uuid.UUID `json:"user_id"`
		}
		if json.Unmarshal([]byte(payload), &m) != nil {
			return
		}
		ev := Event{Type: m.Type, Data: json.RawMessage(`{}`)}
		if m.UserID != nil {
			ev.User = *m.UserID
		}
		h.broadcast(m.ProjectID, ev)
	case ChannelPresence:
		var m presenceMsg
		if json.Unmarshal([]byte(payload), &m) != nil {
			return
		}
		project, err := uuid.Parse(m.ProjectID)
		if err != nil || m.ClientID == "" {
			return
		}
		h.updatePresence(project, m)
	}
}

// presenceKey scopes a tab's entry to its user, so a client id copied from someone else's report cannot touch
// their entry.
func presenceKey(m presenceMsg) string {
	return m.UserID + "/" + m.ClientID
}

func (h *Hub) updatePresence(project uuid.UUID, m presenceMsg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.project(project)
	key := presenceKey(m)
	if m.Leave {
		if _, ok := p.presence[key]; !ok {
			return
		}
		delete(p.presence, key)
	} else {
		e := m.Presence
		e.expires = time.Now().Add(presenceTTL)
		prev, had := p.presence[key]
		p.presence[key] = e
		if had && prev.Route == e.Route && string(prev.Selection) == string(e.Selection) && prev.Email == e.Email {
			return
		}
	}
	h.presenceChangedLocked(project, p)
}

// presenceChangedLocked sends the list at most once per throttle window; a change inside the window is sent at
// its end.
func (h *Hub) presenceChangedLocked(project uuid.UUID, p *projectState) {
	if p.pending {
		return
	}
	wait := presenceThrottle - time.Since(p.lastOut)
	if wait <= 0 {
		p.lastOut = time.Now()
		h.sendLocked(p, Event{Type: EventPresence, Data: presenceJSON(p)})
		return
	}
	p.pending = true
	time.AfterFunc(wait, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		p.pending = false
		p.lastOut = time.Now()
		if h.projects[project] == p {
			h.sendLocked(p, Event{Type: EventPresence, Data: presenceJSON(p)})
		}
	})
}

// sweep expires presence of tabs that stopped reporting.
func (h *Hub) sweep(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now()
		h.mu.Lock()
		for id, p := range h.projects {
			changed := false
			for c, e := range p.presence {
				if now.After(e.expires) {
					delete(p.presence, c)
					changed = true
				}
			}
			if changed {
				h.presenceChangedLocked(id, p)
			}
			if len(p.subs) == 0 && len(p.presence) == 0 && !p.pending {
				delete(h.projects, id)
			}
		}
		h.mu.Unlock()
	}
}
