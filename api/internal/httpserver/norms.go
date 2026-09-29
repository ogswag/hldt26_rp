package httpserver

import (
	"math"
	"net/http"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/econ"
)

func (s *Server) getNorms(w http.ResponseWriter, r *http.Request) {
	if s.q == nil {
		writeJSON(w, http.StatusOK, econ.DefaultNorms())
		return
	}
	n, _, err := catalogstore.LoadNorms(r.Context(), s.q)
	if err != nil {
		s.internal(w, r, "norms.get", err, "Не удалось загрузить нормативы. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) adminGetNorms(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	s.getNorms(w, r)
}

func (s *Server) adminPutNorms(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body econ.Norms
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if msg := normsRangeError(body); msg != "" {
		writeError(w, r, http.StatusBadRequest, msg)
		return
	}
	id := auth.FromRequest(r)
	uid := id.UserID
	if err := s.inTx(r.Context(), func(tx *Server) error {
		if err := catalogstore.SaveNorms(r.Context(), tx.q, body, &uid); err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &uid, Action: audit.ActionNormsUpdate, TargetType: audit.TargetNorms, TargetID: "1",
		})
	}); err != nil {
		s.internal(w, r, "norms.put", err, "Не удалось сохранить нормативы. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func normsRangeError(n econ.Norms) string {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	share := func(v float64) bool { return finite(v) && v >= 0 && v <= 1 }
	// NOTE: availability and utilization divide the throughput in the fleet size, so zero is refused like the
	// assumption set refuses it.
	rate := func(v float64) bool { return finite(v) && v >= 0.01 && v <= 1 }
	pos := func(v float64) bool { return finite(v) && v > 0 }
	nonneg := func(v float64) bool { return finite(v) && v >= 0 }
	switch {
	case !rate(n.Availability) || !rate(n.Utilization):
		return "Доступность и загрузка должны быть от 1% до 100%."
	case !share(n.Reserve):
		return "Резерв должен быть от 0% до 100%."
	case !share(n.InfraFrac) || !share(n.SoftwareFrac) || !share(n.IntegrationYesFrac) || !share(n.IntegrationNoFrac):
		return "Доли CAPEX должны быть от 0% до 100%."
	case !share(n.CommissioningFrac) || !share(n.TrainingFrac) || !share(n.ContingencyFrac) || !share(n.DeliveryFrac):
		return "Доли CAPEX должны быть от 0% до 100%."
	case !pos(n.EnergyKW) || !nonneg(n.EnergyRubPerKWh):
		return "Энергия: мощность больше 0, цена не меньше 0."
	case !nonneg(n.LicensePerRobot) || !nonneg(n.ConsumablePerRobot) || !nonneg(n.CommRubPerRobotYear):
		return "Лицензии, расходники и связь не могут быть отрицательными."
	case !pos(n.RobotsPerTechnician) || !nonneg(n.TechnicianWageMonthRub):
		return "Число роботов на техника больше 0, зарплата не меньше 0."
	case !share(n.DefaultServiceFrac) || !pos(n.DefaultLifetimeYears):
		return "Доля сервиса от 0% до 100%, срок службы больше 0."
	case !share(n.BatteryFrac) || !pos(n.BatteryYears):
		return "Доля батареи от 0% до 100%, срок батареи больше 0."
	case !share(n.RaasMonthlyFrac) || !share(n.RaasMixFixedShare):
		return "Ставки RaaS должны быть от 0% до 100%."
	case !share(n.VATRate) || !rate(n.DiscountRate):
		return "НДС от 0% до 100%, ставка дисконтирования от 1% до 100%."
	case !pos(n.RobotsPerCharger):
		return "Число роботов на зарядку без ТТХ должно быть больше 0."
	default:
		return ""
	}
}
