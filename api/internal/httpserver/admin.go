package httpserver

import (
	"net/http"

	"moscow_hackathon_2026/api/internal/auth"
)

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	id := auth.FromRequest(r)
	if id.IsGuest() {
		writeError(w, r, http.StatusForbidden, "Войдите как администратор.")
		return false
	}
	if !id.IsAdmin() {
		writeError(w, r, http.StatusForbidden, "Нужна роль администратора.")
		return false
	}
	return true
}
