package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/objects"
)

func numericPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

type errorBody struct {
	Error     string               `json:"error"`
	Code      string               `json:"code,omitempty"`
	RequestID string               `json:"request_id,omitempty"`
	Details   []objects.FieldError `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func filledValidated(objectType string, raw json.RawMessage) (json.RawMessage, error) {
	out, err := objects.FillDefaults(objectType, raw)
	if err != nil {
		return nil, err
	}
	if err := objects.Validate(objectType, out); err != nil {
		return nil, err
	}
	return out, nil
}

func rejectParams(w http.ResponseWriter, r *http.Request, err error) {
	if writeValidateErr(w, r, err) {
		return
	}
	writeError(w, r, http.StatusBadRequest, "Параметры не прошли проверку. Исправьте поля и повторите.")
}

// echoRequestID returns the request id in a header too, so a client can name a request whose body it cannot read.
func echoRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

func requestID(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg, RequestID: middleware.GetReqID(r.Context())})
}

func writeCode(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code, RequestID: middleware.GetReqID(r.Context())})
}

func writeValidation(w http.ResponseWriter, r *http.Request, ve *objects.ValidationError) {
	writeJSON(w, http.StatusBadRequest, errorBody{
		Error:     "Параметры не прошли проверку. Исправьте указанные поля и повторите.",
		Code:      "validation_error",
		RequestID: middleware.GetReqID(r.Context()),
		Details:   ve.Details,
	})
}

func writeValidateErr(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, objects.ErrUnknownType) {
		writeError(w, r, http.StatusBadRequest, "Тип объекта должен быть warehouse, airport или hospital.")
		return true
	}
	var ve *objects.ValidationError
	if errors.As(err, &ve) {
		writeValidation(w, r, ve)
		return true
	}
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool) bool {
	return decodeJSONLimit(w, r, dst, allowEmpty, 1<<20)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if errors.Is(err, io.EOF) && allowEmpty {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "Запрос слишком большой. Разбейте его на части.")
		return false
	}
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный JSON. Проверьте тело запроса.")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "Тело запроса должно содержать один JSON-объект.")
		return false
	}
	return true
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func floatToNumeric(p *float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if p == nil {
		return n, nil
	}
	s := strconv.FormatFloat(*p, 'f', -1, 64)
	if err := n.Scan(s); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "op", "http.recover", "request_id", middleware.GetReqID(r.Context()), "panic", rec)
				writeError(w, r, http.StatusInternalServerError, "Внутренняя ошибка. Сообщите идентификатор запроса.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		s.log.Info("http",
			"op", "http",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"ms", time.Since(start).Milliseconds(),
		)
	})
}
