package httpapi

import (
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"net/http"
)

func (s *server) monthlyDraftRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/monthly-drafts", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.MonthlyDraft { return st.MonthlyDrafts }, func(d domain.MonthlyDraft) bool {
			return (r.URL.Query().Get("month") == "" || d.Month == r.URL.Query().Get("month")) && d.State == "pending"
		})
	})
	m.HandleFunc("PUT /api/v1/monthly-drafts/{id}", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.MonthlyDraftEdit, v int64) (any, int64, error) {
			d, err := st.EditMonthlyDraft(r.PathValue("id"), in, v)
			return d, d.Version, err
		})
	})
	m.HandleFunc("POST /api/v1/monthly-drafts/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, _ struct{}, v int64) (any, int64, error) {
			d, err := st.RejectMonthlyDraft(r.PathValue("id"), v)
			return d, d.Version, err
		})
	})
	m.HandleFunc("POST /api/v1/monthly-drafts/{id}/accept", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.MonthlyDraftAccept, v int64) (any, int64, error) {
			sheet, err := st.AcceptMonthlyDraft(r.PathValue("id"), v, in.SheetVersion)
			return sheet, sheet.Version, err
		})
	})
}
