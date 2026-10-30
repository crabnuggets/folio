package httpapi

import (
	"net/http"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

func monthlyMutate[T any](s *server, w http.ResponseWriter, r *http.Request, versioned bool, fn func(*domain.State, T, int64) (domain.MonthlySheet, error)) {
	mutate(s, w, r, 200, versioned, func(st *domain.State, in T, v int64) (any, int64, error) {
		sheet, err := fn(st, in, v)
		return sheet, sheet.Version, err
	})
}

func (s *server) monthlyRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/monthly-sheets/{month}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			sheet, err := st.GetMonthlySheet(r.PathValue("month"))
			return sheet, sheet.Version, err
		})
	})
	m.HandleFunc("POST /api/v1/monthly-sheets/{month}/open", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, false, func(st *domain.State, _ struct{}, _ int64) (domain.MonthlySheet, error) {
			return st.OpenMonthlySheet(r.PathValue("month"))
		})
	})
	m.HandleFunc("POST /api/v1/monthly-sheets/{month}/items", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlyItemInput, v int64) (domain.MonthlySheet, error) {
			return st.AddMonthlyItem(r.PathValue("month"), in, v)
		})
	})
	m.HandleFunc("PUT /api/v1/monthly-sheets/{month}/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlyItemUpdate, v int64) (domain.MonthlySheet, error) {
			return st.UpdateMonthlyItem(r.PathValue("month"), r.PathValue("id"), in, v)
		})
	})
	for _, action := range []string{"remove", "restore"} {
		m.HandleFunc("POST /api/v1/monthly-sheets/{month}/items/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			monthlyMutate(s, w, r, true, func(st *domain.State, _ struct{}, v int64) (domain.MonthlySheet, error) {
				return st.RemoveMonthlyItem(r.PathValue("month"), r.PathValue("id"), action == "remove", v)
			})
		})
	}
	m.HandleFunc("POST /api/v1/monthly-sheets/{month}/usual-amounts", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlyUsualInput, v int64) (domain.MonthlySheet, error) {
			return st.UseMonthlyUsualAmounts(r.PathValue("month"), in, v)
		})
	})
	m.HandleFunc("POST /api/v1/monthly-sheets/{month}/undo", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlyUndoInput, v int64) (domain.MonthlySheet, error) {
			return st.UndoMonthlySheet(r.PathValue("month"), in, v)
		})
	})
	m.HandleFunc("POST /api/v1/monthly-sheets/{month}/sections", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlySectionInput, v int64) (domain.MonthlySheet, error) {
			return st.CreateMonthlySection(r.PathValue("month"), in, v)
		})
	})
	m.HandleFunc("PUT /api/v1/monthly-sheets/{month}/sections/{id}", func(w http.ResponseWriter, r *http.Request) {
		monthlyMutate(s, w, r, true, func(st *domain.State, in domain.MonthlySectionRename, v int64) (domain.MonthlySheet, error) {
			return st.RenameMonthlySection(r.PathValue("month"), r.PathValue("id"), in, v)
		})
	})
	m.HandleFunc("GET /api/v1/monthly-item-suggestions", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			result, err := st.SuggestMonthlyItems(r.URL.Query().Get("month"), r.URL.Query().Get("section_id"), r.URL.Query().Get("q"))
			return result, 0, err
		})
	})
}
