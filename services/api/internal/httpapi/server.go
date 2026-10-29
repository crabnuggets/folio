// Package httpapi serves the common web and Android REST contract.
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/crabnuggets/folio/services/api/internal/store"
)

const MaxBodyBytes int64 = 1 << 20

type server struct {
	store store.Store
	owner string
}
type errorEnvelope struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields"`
		RequestID string            `json:"request_id"`
	} `json:"error"`
}

// New builds the API for ownerID with public liveness and database-readiness routes.
// middleware wraps /api/v1/; nil denies all API access. Responses receive request
// and security headers, and recovered panics produce a public internal error.
func New(db store.Store, ownerID string, middleware func(http.Handler) http.Handler) http.Handler {
	return newServer(db, ownerID, middleware, "")
}

// NewWithWebDistDir builds the API and optionally serves a built React SPA from
// webDistDir. An empty path preserves API-only behavior. A configured directory
// must exist and contain a regular index.html; static files are opened beneath
// that directory and never replace API or health responses.
func NewWithWebDistDir(db store.Store, ownerID string, middleware func(http.Handler) http.Handler, webDistDir string) (http.Handler, error) {
	if strings.TrimSpace(webDistDir) == "" {
		return New(db, ownerID, middleware), nil
	}
	root, err := os.OpenRoot(webDistDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("WEB_DIST_DIR must be a directory")
	}
	index, err := root.Open("index.html")
	if err != nil {
		return nil, errors.New("WEB_DIST_DIR must contain index.html")
	}
	indexInfo, err := index.Stat()
	_ = index.Close()
	if err != nil || !indexInfo.Mode().IsRegular() {
		return nil, errors.New("WEB_DIST_DIR index.html must be a regular file")
	}
	return newServer(db, ownerID, middleware, webDistDir), nil
}

func newServer(db store.Store, ownerID string, middleware func(http.Handler) http.Handler, webDistDir string) http.Handler {
	s := &server{db, ownerID}
	api := http.NewServeMux()
	s.routes(api)
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, r, domain.NotFound()) })
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, 200, map[string]string{"status": "ok"}, 0)
	})
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeError(w, r, domain.Fail(503, "UNAVAILABLE", "Database is unavailable"))
			return
		}
		writeJSON(w, r, 200, map[string]string{"status": "ready"}, 0)
	})
	if middleware == nil {
		middleware = func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeError(w, r, domain.Fail(401, "UNAUTHENTICATED", "Authentication is required"))
			})
		}
	}
	root.Handle("/api/v1/", middleware(api))
	root.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if webDistDir != "" {
			serveWeb(w, r, webDistDir)
			return
		}
		writeError(w, r, domain.NotFound())
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", domain.NewID())
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if recover() != nil {
				writeError(w, r, domain.Fail(500, "INTERNAL_ERROR", "The request could not be completed"))
			}
		}()
		if webDistDir != "" && unsafeWebPath(r.URL.Path) {
			writeError(w, r, domain.NotFound())
			return
		}
		root.ServeHTTP(w, r)
	})
}

// writeError sends domain error details with the response's request ID.
// Other errors become a generic 503 response without exposing their contents.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		de = &domain.Error{Status: 503, Code: "UNAVAILABLE", Message: "The request could not be completed"}
	}
	v := errorEnvelope{}
	v.Error.Code = de.Code
	v.Error.Message = de.Message
	v.Error.Fields = de.Fields
	if v.Error.Fields == nil {
		v.Error.Fields = map[string]string{}
	}
	v.Error.RequestID = w.Header().Get("X-Request-ID")
	writeJSON(w, r, de.Status, v, 0)
}

// writeJSON sends a JSON response with an ETag for positive versions.
// Encoding failures become a public 500 error response.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any, version int64) {
	result, err := store.JSON(status, v, version)
	if err != nil {
		writeError(w, r, domain.Fail(500, "INTERNAL_ERROR", "Unable to encode response"))
		return
	}
	writeResult(w, result)
}

// writeResult sends the encoded status, body, and optional ETag, ignoring write errors.
func writeResult(w http.ResponseWriter, result store.Result) {
	w.Header().Set("Content-Type", "application/json")
	if result.ETag != "" {
		w.Header().Set("ETag", result.ETag)
	}
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}

// readJSON decodes one request object, checking required fields and rejecting null,
// unknown fields, incompatible values, and trailing JSON with a 400 MALFORMED_JSON
// error. Draft candidate completeness is deferred until acceptance.
func readJSON[T any](body []byte) (T, error) {
	var in T
	if err := validateRequired(body, reflect.TypeOf(in)); err != nil {
		return in, err
	}
	d := json.NewDecoder(strings.NewReader(string(body)))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		return in, domain.Fail(400, "MALFORMED_JSON", "Expected a JSON object matching the request schema")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return in, domain.Fail(400, "MALFORMED_JSON", "Expected exactly one JSON object")
	}
	if len(body) == 0 || strings.TrimSpace(string(body)) == "null" {
		return in, domain.Fail(400, "MALFORMED_JSON", "A JSON object is required")
	}
	return in, nil
}

// version reads a strong If-Match ETag containing a canonical positive int64.
// Missing preconditions return 428; malformed or out-of-range versions return 400.
func version(r *http.Request) (int64, error) {
	v := r.Header.Get("If-Match")
	if v == "" {
		return 0, domain.Fail(428, "PRECONDITION_REQUIRED", "If-Match is required")
	}
	if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' {
		return 0, domain.Fail(400, "INVALID_PRECONDITION", "If-Match must contain a quoted integer version")
	}
	raw := v[1 : len(v)-1]
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n < 1 || strconv.FormatInt(n, 10) != raw {
		return 0, domain.Fail(400, "INVALID_PRECONDITION", "If-Match must contain a positive integer version")
	}
	return n, nil
}

// read loads owner state and sends fn's value with status 200 and its optional
// version ETag. Store and callback errors are sent through writeError.
func (s *server) read(w http.ResponseWriter, r *http.Request, fn func(*domain.State) (any, int64, error)) {
	state, e := s.store.Read(r.Context(), s.owner)
	if e != nil {
		writeError(w, r, e)
		return
	}
	v, ver, e := fn(state)
	if e != nil {
		writeError(w, r, e)
		return
	}
	writeJSON(w, r, 200, v, ver)
}

// mutate validates a UUID retry key, JSON media type, a body of at most 1 MiB,
// and If-Match when versioned. It executes fn transactionally, validates summaries,
// and stores the encoded response for retries. Request identity includes method,
// URI, If-Match, and exact body bytes; replayed responses carry Idempotency-Replayed.
// Request, callback, summary, storage, and encoding errors are sent through writeError.
func mutate[T any](s *server, w http.ResponseWriter, r *http.Request, status int, versioned bool, fn func(*domain.State, T, int64) (any, int64, error)) {
	key := r.Header.Get("Idempotency-Key")
	if !domain.ValidID(key) {
		writeError(w, r, domain.Fail(400, "INVALID_IDEMPOTENCY_KEY", "A UUID Idempotency-Key is required"))
		return
	}
	if media := strings.Split(r.Header.Get("Content-Type"), ";")[0]; media != "application/json" {
		writeError(w, r, domain.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json"))
		return
	}
	var expected int64
	var e error
	if versioned {
		expected, e = version(r)
		if e != nil {
			writeError(w, r, e)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	body, e := io.ReadAll(r.Body)
	if e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			writeError(w, r, domain.Fail(413, "BODY_TOO_LARGE", "Request body exceeds 1 MiB"))
		} else {
			writeError(w, r, domain.Fail(400, "MALFORMED_BODY", "Could not read request body"))
		}
		return
	}
	in, e := readJSON[T](body)
	if e != nil {
		writeError(w, r, e)
		return
	}
	hash := sha256.Sum256([]byte(r.Method + "\n" + r.URL.RequestURI() + "\n" + r.Header.Get("If-Match") + "\n" + string(body)))
	result, replay, e := s.store.Execute(r.Context(), s.owner, key, hex.EncodeToString(hash[:]), func(state *domain.State) (store.Result, error) {
		v, ver, err := fn(state, in, expected)
		if err != nil {
			return store.Result{}, err
		}
		if err := state.ValidateSummaries(); err != nil {
			return store.Result{}, err
		}
		return store.JSON(status, v, ver)
	})
	if e != nil {
		writeError(w, r, e)
		return
	}
	if replay {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeResult(w, result)
}

type page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// paginate returns matching values in lexicographic key order after an exclusive
// UUID cursor. The limit defaults to 50 and must be 1 through 100; invalid limits
// or cursors return INVALID_PAGINATION. NextCursor is nil on the last page.
func paginate[T any](r *http.Request, values map[string]T, filter func(T) bool) (page[T], error) {
	p := page[T]{Items: []T{}}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			return p, domain.Fail(400, "INVALID_PAGINATION", "limit must be between 1 and 100")
		}
		limit = v
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" && !domain.ValidID(cursor) {
		return p, domain.Fail(400, "INVALID_PAGINATION", "cursor must be a resource UUID")
	}
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if k > cursor && (filter == nil || filter(v)) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i >= limit {
			last := keys[i-1]
			p.NextCursor = &last
			break
		}
		p.Items = append(p.Items, values[k])
	}
	return p, nil
}

// list sends a filtered page of owner records, rejecting an invalid optional month
// query with INVALID_MONTH. The caller's filter determines month membership;
// storage and pagination errors are sent through writeError.
func list[T any](s *server, w http.ResponseWriter, r *http.Request, fn func(*domain.State) map[string]T, filter func(T) bool) {
	if month := r.URL.Query().Get("month"); month != "" && !domain.ValidMonth(month) {
		writeError(w, r, domain.Fail(400, "INVALID_MONTH", "month must be YYYY-MM"))
		return
	}
	s.read(w, r, func(state *domain.State) (any, int64, error) { v, e := paginate(r, fn(state), filter); return v, 0, e })
}

// lookup returns the record named by the id path parameter, or NOT_FOUND if absent.
func lookup[T any](r *http.Request, values map[string]T) (T, error) {
	id := r.PathValue("id")
	v, ok := values[id]
	if !ok {
		return v, domain.NotFound()
	}
	return v, nil
}

// routes registers financial reads, mutations, export, and deferred integration
// endpoints, which respond with 501 FEATURE_NOT_AVAILABLE.
func (s *server) routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { return st.Settings, st.Settings.Version, nil })
	})
	m.HandleFunc("PATCH /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.SettingsInput, v int64) (any, int64, error) {
			o, e := st.UpdateSettings(in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Account { return st.Accounts }, nil)
	})
	m.HandleFunc("POST /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.AccountInput, _ int64) (any, int64, error) {
			o, e := st.CreateAccount(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { o, e := lookup(r, st.Accounts); return o, o.Version, e })
	})
	m.HandleFunc("PATCH /api/v1/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.AccountInput, v int64) (any, int64, error) {
			o, e := st.UpdateAccount(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/entries", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Entry { return st.Entries }, func(v domain.Entry) bool {
			return r.URL.Query().Get("month") == "" || v.ReportingMonth == r.URL.Query().Get("month")
		})
	})
	m.HandleFunc("POST /api/v1/entries", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.EntryInput, _ int64) (any, int64, error) {
			o, e := st.CreateEntry(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/entries/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { o, e := lookup(r, st.Entries); return o, o.Version, e })
	})
	m.HandleFunc("POST /api/v1/entries/{id}/corrections", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.EntryCorrection, v int64) (any, int64, error) {
			o, e := st.CorrectEntry(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/statements", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Statement { return st.Statements }, func(v domain.Statement) bool {
			return r.URL.Query().Get("month") == "" || v.ReportingMonth == r.URL.Query().Get("month")
		})
	})
	m.HandleFunc("POST /api/v1/statements", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.StatementInput, _ int64) (any, int64, error) {
			o, e := st.CreateStatement(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/statements/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { o, e := lookup(r, st.Statements); return o, o.Version, e })
	})
	m.HandleFunc("POST /api/v1/statements/{id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, true, func(st *domain.State, in domain.StatementRevision, v int64) (any, int64, error) {
			o, e := st.ReviseStatement(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/statements/{id}/settlements", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, true, func(st *domain.State, in domain.SettlementInput, v int64) (any, int64, error) {
			o, e := st.CreateSettlement(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/statements/{id}/obligation-adjustments", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, true, func(st *domain.State, in domain.ObligationAdjustmentInput, v int64) (any, int64, error) {
			o, e := st.CreateObligationAdjustment(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/balance-snapshots", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Snapshot { return st.Snapshots }, nil)
	})
	m.HandleFunc("POST /api/v1/balance-snapshots", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.SnapshotInput, _ int64) (any, int64, error) {
			o, e := st.CreateSnapshot(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/claims", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Claim { return st.Claims }, nil)
	})
	m.HandleFunc("POST /api/v1/claims", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.ClaimInput, _ int64) (any, int64, error) {
			o, e := st.CreateClaim(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/claims/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { o, e := lookup(r, st.Claims); return o, o.Version, e })
	})
	m.HandleFunc("PATCH /api/v1/claims/{id}", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.ClaimInput, v int64) (any, int64, error) {
			o, e := st.UpdateClaim(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/claims/{id}/receipt-matches", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, true, func(st *domain.State, in domain.ReceiptMatchInput, v int64) (any, int64, error) {
			o, e := st.MatchReceipt(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/months/{month}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			o, e := st.GetMonth(r.PathValue("month"))
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/months/{month}/plan", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			o, e := st.GetPlan(r.PathValue("month"))
			return o, o.Version, e
		})
	})
	m.HandleFunc("PUT /api/v1/months/{month}/plan", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.PlanInput, v int64) (any, int64, error) {
			o, e := st.PutPlan(r.PathValue("month"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/months/{month}/summary", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			o, e := st.Summarize(r.PathValue("month"))
			return o, o.Revision, e
		})
	})
	m.HandleFunc("POST /api/v1/months/{month}/close", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.MonthTransition, v int64) (any, int64, error) {
			o, e := st.CloseMonth(r.PathValue("month"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/months/{month}/reopen", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in domain.MonthTransition, v int64) (any, int64, error) {
			o, e := st.ReopenMonth(r.PathValue("month"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/months/{month}/insights", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			o, e := st.Insights(r.PathValue("month"))
			return map[string]any{"items": o}, 0, e
		})
	})
	m.HandleFunc("GET /api/v1/recurrences", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Recurrence { return st.Recurrences }, nil)
	})
	m.HandleFunc("POST /api/v1/recurrences", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.RecurrenceInput, _ int64) (any, int64, error) {
			o, e := st.CreateRecurrence(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/recurrences/{id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, true, func(st *domain.State, in domain.RecurrenceInput, v int64) (any, int64, error) {
			o, e := st.ReviseRecurrence(r.PathValue("id"), in, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/reconciliation-periods", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Period { return st.Periods }, nil)
	})
	m.HandleFunc("POST /api/v1/reconciliation-periods", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.PeriodInput, _ int64) (any, int64, error) {
			o, e := st.CreatePeriod(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Draft { return st.Drafts }, nil)
	})
	m.HandleFunc("POST /api/v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 201, false, func(st *domain.State, in domain.DraftInput, _ int64) (any, int64, error) {
			o, e := st.CreateDraft(in)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/drafts/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.read(w, r, func(st *domain.State) (any, int64, error) { o, e := lookup(r, st.Drafts); return o, o.Version, e })
	})
	m.HandleFunc("POST /api/v1/drafts/{id}/accept", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in struct{}, v int64) (any, int64, error) {
			o, e := st.AcceptDraft(r.PathValue("id"), v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("POST /api/v1/drafts/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		mutate(s, w, r, 200, true, func(st *domain.State, in struct {
			Reason string `json:"reason"`
		}, v int64) (any, int64, error) {
			o, e := st.RejectDraft(r.PathValue("id"), in.Reason, v)
			return o, o.Version, e
		})
	})
	m.HandleFunc("GET /api/v1/audit-events", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.Audit {
			o := map[string]domain.Audit{}
			for _, v := range st.AuditEvents {
				o[v.ID] = v
			}
			return o
		}, nil)
	})
	m.HandleFunc("GET /api/v1/legacy-month-summaries", func(w http.ResponseWriter, r *http.Request) {
		list(s, w, r, func(st *domain.State) map[string]domain.LegacySummary {
			o := map[string]domain.LegacySummary{}
			for _, v := range st.LegacySummaries {
				o[v.ID] = v
			}
			return o
		}, nil)
	})
	m.HandleFunc("GET /api/v1/exports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="folio-export-v1.json"`)
		s.read(w, r, func(st *domain.State) (any, int64, error) {
			return struct {
				SchemaVersion int           `json:"schema_version"`
				ExportedAt    time.Time     `json:"exported_at"`
				State         *domain.State `json:"state"`
			}{1, time.Now().UTC(), st}, 0, nil
		})
	})
	for _, pattern := range []string{"POST /api/v1/imports", "GET /api/v1/imports/{id}", "POST /api/v1/gmail/connect", "GET /api/v1/gmail/callback", "POST /api/v1/gmail/sync", "DELETE /api/v1/gmail/connection", "POST /api/v1/capture/text"} {
		m.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, domain.Fail(501, "FEATURE_NOT_AVAILABLE", "This integration is defined for a later implementation phase"))
		})
	}
}
