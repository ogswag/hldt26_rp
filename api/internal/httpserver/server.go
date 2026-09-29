package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/realtime"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simjobs"
)

const solutionsCap = 500

type Server struct {
	cfg          config.Config
	log          *slog.Logger
	q            *db.Queries
	sims         *simjobs.Service
	queue        Notifier
	scopedRoutes map[string]accessLevel
	dummyHash    func() string
	hub          *realtime.Hub
	photos       *importers.PhotoFetcher
	heartbeat    time.Duration
	// lockedSeq is draft_seq under the project lock, set only on the copy inside inProjectTx.
	lockedSeq int64
}

// New builds the router. sims and queue may be nil when simulations are not served, hub when live project
// streams are not.
func New(cfg config.Config, log *slog.Logger, q *db.Queries, sims *simjobs.Service, queue Notifier, hub *realtime.Hub) http.Handler {
	_, r := newServer(cfg, log, q, sims, queue, hub)
	return r
}

func newServer(cfg config.Config, log *slog.Logger, q *db.Queries, sims *simjobs.Service, queue Notifier, hub *realtime.Hub) (*Server, chi.Router) {
	if cfg.SessionIdle <= 0 {
		cfg.SessionIdle = 168 * time.Hour
	}
	if cfg.SessionMax <= 0 {
		cfg.SessionMax = 30 * 24 * time.Hour
	}
	if cfg.BcryptCost < 10 {
		cfg.BcryptCost = 10
	}
	if cfg.SimActivePerUser <= 0 {
		cfg.SimActivePerUser = 3
	}
	s := &Server{
		cfg: cfg, log: log, q: q, sims: sims, queue: queue, hub: hub, scopedRoutes: map[string]accessLevel{},
		photos: importers.NewPhotoFetcher(false),
	}
	s.dummyHash = sync.OnceValue(func() string {
		h, err := auth.HashPassword("no-such-account", cfg.BcryptCost)
		if err != nil {
			log.Error("auth.dummy_hash", "op", "auth.login", "err", err)
		}
		return h
	})
	if sims == nil || queue == nil {
		s.queue = nil
	}
	root := chi.NewRouter()
	root.Use(middleware.RequestID)
	root.Use(echoRequestID)
	root.Use(s.requestLogger)
	root.Use(s.recoverer)
	root.Use(s.identity)
	root.Use(s.csrf)
	root.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeCode(w, r, http.StatusNotFound, "not_found", "Такого адреса в API нет. Проверьте ссылку.")
	})
	root.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeCode(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Этот адрес API не принимает такой запрос.")
	})
	// Streams live for hours; everything else gets the request timeout.
	s.scoped(root, http.MethodGet, "/api/projects/{id}/events", accessRead, s.projectEvents)
	r := root.With(middleware.Timeout(30 * time.Second))

	r.Get("/health", s.health)
	r.Get("/version", s.version)
	r.Get("/api/solutions", s.listSolutions)
	r.Get("/api/solutions/{solutionId}", s.getSolution)
	r.Get("/api/solutions/{solutionId}/image", s.solutionImage)
	r.Get("/api/engine/*", s.engineFiles)
	r.Get("/api/catalog/bundle", s.catalogBundle)
	r.Get("/api/catalog/dictionaries", s.catalogDictionaries)
	r.Get("/api/object-types", s.listObjectTypes)
	r.Get("/api/object-types/{type}/schema", s.getObjectSchema)
	r.Post("/api/auth/register", s.register)
	r.Post("/api/auth/login", s.login)
	r.Get("/api/auth/me", s.me)
	r.Post("/api/auth/logout", s.logout)
	r.Post("/api/auth/verify-email", s.verifyEmail)
	r.Post("/api/auth/verify-email/resend", s.resendVerification)
	r.Post("/api/auth/password-reset/request", s.requestPasswordReset)
	r.Post("/api/auth/password-reset/confirm", s.confirmPasswordReset)
	r.Post("/api/auth/invitations/inspect", s.inspectInvitation)
	r.Get("/api/dev/outbox", s.devOutbox)
	r.Get("/api/auth/sessions", s.listSessions)
	r.Delete("/api/auth/sessions/{sessionId}", s.revokeSession)
	r.Post("/api/auth/sessions/revoke-others", s.revokeOtherSessions)
	r.Post("/api/guest/calculate", s.guestCalculate)
	r.Post("/api/guest/match", s.guestMatch)
	r.Post("/api/guest/export", s.guestExport)
	r.Post("/api/projects", s.createProject)
	r.Post("/api/projects/import", s.importProject)
	r.Get("/api/projects", s.listProjects)
	r.Get("/api/projects/trash", s.listTrash)

	get, post, put, del := http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete
	s.scoped(r, get, "/api/projects/{id}", accessRead, s.getProject)
	s.scoped(r, post, "/api/projects/{id}/transactions", accessWrite, s.postTransactions)
	s.scoped(r, get, "/api/projects/{id}/snapshot", accessRead, s.getSnapshot)
	s.scoped(r, get, "/api/projects/{id}/operations", accessRead, s.listOperations)
	s.scoped(r, post, "/api/projects/{id}/presence", accessRead, s.postPresence)
	s.scoped(r, del, "/api/projects/{id}", accessOwner, s.deleteProject)
	s.scoped(r, post, "/api/projects/{id}/copy", accessWrite, s.copyProject)
	s.scoped(r, post, "/api/projects/{id}/calculate", accessWrite, s.calculateProject)
	s.scoped(r, post, "/api/projects/{id}/calculations", accessWrite, s.calculateProject)
	s.scoped(r, get, "/api/projects/{id}/calculations", accessRead, s.listCalculations)
	s.scoped(r, get, "/api/projects/{id}/runs", accessRead, s.listRuns)
	s.scoped(r, get, "/api/projects/{id}/sim-checks", accessRead, s.getSimChecks)
	s.scoped(r, get, "/api/calculations/{runId}", accessRead, s.getCalculation)
	s.scoped(r, post, "/api/calculations/{runId}/export", accessRead, s.exportCalculation)
	s.scoped(r, get, "/api/projects/{id}/processes", accessRead, s.listProcesses)
	s.scoped(r, get, "/api/projects/{id}/variants", accessRead, s.listVariants)
	s.scoped(r, get, "/api/projects/{id}/shared-costs", accessRead, s.listSharedCosts)
	s.scoped(r, get, "/api/projects/{id}/assumption-sets", accessRead, s.listAssumptionSets)
	s.scoped(r, get, "/api/projects/{id}/versions", accessRead, s.listVersions)
	s.scoped(r, get, "/api/projects/{id}/versions/{versionId}", accessRead, s.getVersion)
	s.scoped(r, post, "/api/projects/{id}/map/validate", accessRead, s.validateProjectMap)
	s.scoped(r, get, "/api/projects/{id}/map/template", accessRead, s.projectMapTemplate)
	s.scoped(r, post, "/api/projects/{id}/simulations", accessWrite, s.createSimulation)
	s.scoped(r, get, "/api/projects/{id}/simulations", accessRead, s.listSimulations)
	s.scoped(r, get, "/api/simulation-jobs/{jobId}", accessRead, s.getSimulationJob)
	s.scoped(r, del, "/api/simulation-jobs/{jobId}", accessWrite, s.cancelSimulationJob)
	s.scoped(r, post, "/api/simulation-jobs/{jobId}/retry", accessWrite, s.retrySimulationJob)
	s.scoped(r, get, "/api/simulation-runs/{runId}", accessRead, s.getSimulationRun)
	s.scoped(r, get, "/api/simulation-runs/{runId}/events", accessRead, s.getSimulationEvents)
	s.scoped(r, put, "/api/simulation-runs/{runId}/pin", accessWrite, s.pinSimulationRun)
	s.scoped(r, post, "/api/simulation-runs/{runId}/export", accessRead, s.exportSimulationRun)
	s.scopedTrash(r, post, "/api/projects/{id}/restore", s.restoreProject)
	s.scopedTrash(r, del, "/api/projects/{id}/purge", s.purgeProject)
	s.scoped(r, get, "/api/projects/{id}/members", accessOwner, s.listMembers)
	s.scoped(r, post, "/api/projects/{id}/members", accessOwner, s.addMember)
	s.scoped(r, put, "/api/projects/{id}/members/{userId}", accessOwner, s.setMemberRole)
	s.scoped(r, del, "/api/projects/{id}/members/{userId}", accessRead, s.removeMember)
	s.scoped(r, get, "/api/projects/{id}/match", accessRead, s.projectMatchPreview)
	s.scoped(r, post, "/api/projects/{id}/export", accessRead, s.projectExport)

	r.Post("/api/admin/solutions", s.adminCreateSolution)
	r.Patch("/api/admin/solutions/{solutionId}", s.adminPatchSolution)
	r.Put("/api/admin/solutions/{solutionId}", s.adminUpdateSolution)
	r.Delete("/api/admin/solutions/{solutionId}", s.adminDeleteSolution)
	r.Post("/api/admin/solutions/{solutionId}/restore", s.adminRestoreSolution)
	r.Post("/api/admin/solutions/{solutionId}/duplicate", s.adminDuplicateSolution)
	r.Put("/api/admin/solutions/{solutionId}/image", s.adminPutSolutionImage)
	r.Delete("/api/admin/solutions/{solutionId}/image", s.adminDeleteSolutionImage)
	r.Get("/api/admin/catalog/export", s.adminExportCatalog)
	r.Get("/api/admin/catalog/template", s.adminCatalogTemplate)
	r.Post("/api/admin/catalog/imports", s.adminCreateImport)
	r.Get("/api/admin/catalog/imports/{importId}", s.adminGetImport)
	r.Patch("/api/admin/catalog/imports/{importId}", s.adminPatchImport)
	r.Delete("/api/admin/catalog/imports/{importId}", s.adminDeleteImport)
	r.Post("/api/admin/catalog/imports/{importId}/apply", s.adminApplyImport)
	r.Get("/api/admin/catalog/imports/{importId}/errors", s.adminImportErrors)
	r.Post("/api/admin/invitations", s.adminCreateInvitation)
	r.Get("/api/admin/invitations", s.adminListInvitations)
	r.Delete("/api/admin/invitations/{inviteId}", s.adminRevokeInvitation)
	r.Get("/api/admin/audit", s.listAuditEvents)
	return s, root
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"api": "0.6.0", "model": econ.ModelVersion, "sim": sim.Version})
}

type specsJSON struct {
	PayloadKg      *float64   `json:"payload_kg"`
	MassKg         *float64   `json:"mass_kg"`
	LengthMm       *float64   `json:"length_mm"`
	WidthMm        *float64   `json:"width_mm"`
	HeightMm       *float64   `json:"height_mm"`
	SpeedMps       *float64   `json:"speed_mps"`
	EnduranceH     *float64   `json:"endurance_h"`
	ChargeMin      *float64   `json:"charge_min"`
	NavType        *string    `json:"nav_type"`
	PosAccuracyMm  *float64   `json:"pos_accuracy_mm"`
	MinAisleMm     *float64   `json:"min_aisle_mm"`
	TurnRadiusMm   *float64   `json:"turn_radius_mm"`
	TempMinC       *float64   `json:"temp_min_c"`
	TempMaxC       *float64   `json:"temp_max_c"`
	LifetimeYears  *float64   `json:"lifetime_years"`
	ServicePctYear *float64   `json:"service_pct_year"`
	Confidence     *string    `json:"confidence"`
	SourcedAt      *time.Time `json:"sourced_at"`
}

type solutionJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Vendor      *string  `json:"vendor"`
	Kind        *string  `json:"kind"`
	Subtype     *string  `json:"subtype"`
	Status      *string  `json:"status"`
	Industry    *string  `json:"industry"`
	Scenario    *string  `json:"scenario"`
	PriceRub    *float64 `json:"price_rub"`
	SourceURL   *string  `json:"source_url"`
	Family      *string  `json:"family"`
	Description *string  `json:"description"`
	ObjectTypes []string `json:"object_types"`
	// FieldSources say where single values come from, by field code; a value with no entry has the source of the card.
	FieldSources map[string]catalog.FieldSource `json:"field_sources"`
	// Modification names the row of a repeated catalog id (its industry); null for a robot the file lists once.
	Modification *string              `json:"modification"`
	Region       *string              `json:"region"`
	UGT          *float64             `json:"ugt"`
	Market       *float64             `json:"market"`
	ArchivedAt   *time.Time           `json:"archived_at"`
	ImageSHA     *string              `json:"image_sha"`
	Specs        specsJSON            `json:"specs"`
	DataQuality  matching.DataQuality `json:"data_quality"`
}

type createProjectBody struct {
	Name       string          `json:"name"`
	ObjectType string          `json:"object_type"`
	Params     json.RawMessage `json:"params"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if guestForbidden(w, r) {
		return
	}
	var body createProjectBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, r, http.StatusBadRequest, "Укажите имя проекта.")
		return
	}
	if !objects.Valid(body.ObjectType) {
		writeError(w, r, http.StatusBadRequest, "Тип объекта должен быть warehouse, airport или hospital.")
		return
	}
	params := body.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(params, &probe); err != nil {
		writeError(w, r, http.StatusBadRequest, "params должен быть JSON-объектом. Проверьте тело запроса.")
		return
	}
	if len(probe) > 0 {
		if err := objects.Validate(body.ObjectType, params); err != nil {
			if writeValidateErr(w, r, err) {
				return
			}
			writeError(w, r, http.StatusBadRequest, "Параметры не прошли проверку. Исправьте поля и повторите.")
			return
		}
	}
	uid := auth.FromRequest(r).UserID
	row, err := s.q.CreateProject(r.Context(), db.CreateProjectParams{
		UserID:       &uid,
		Name:         body.Name,
		ObjectType:   body.ObjectType,
		Params:       params,
		ModelVersion: econ.ModelVersion,
		InputHash:    "",
	})
	if err != nil {
		s.log.Error("projects.create", "op", "projects.create", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось создать проект. Повторите запрос.")
		return
	}
	fresh, err := s.q.GetProject(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.create.reload", "op", "projects.create", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось создать проект. Повторите запрос.")
		return
	}
	p, ok := s.projectJSONFromRow(w, r, fresh)
	if !ok {
		return
	}
	p.Access = RoleOwner
	writeProject(w, http.StatusCreated, p)
}
