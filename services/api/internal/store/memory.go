package store

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

type retryRecord struct {
	Hash   string
	Result Result
}

// Memory is a transactional test store. The server never selects it implicitly.
type Memory struct {
	mu      sync.Mutex
	states  map[string]*domain.State
	retries map[string]map[string]retryRecord
}

// NewMemory creates an empty transactional test store with owner-scoped retry records.
func NewMemory() *Memory {
	return &Memory{states: make(map[string]*domain.State), retries: make(map[string]map[string]retryRecord)}
}
// cloneState returns a detached copy, or a new default state for nil input.
// Serialization failures become DEPENDENCY_UNAVAILABLE errors.
func cloneState(state *domain.State) (*domain.State, error) {
	if state == nil {
		return domain.NewState(), nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, unavailable()
	}
	result := domain.NewState()
	if json.Unmarshal(data, result) != nil {
		return nil, unavailable()
	}
	return result, nil
}
// cloneResult copies the response body so callers cannot mutate a cached retry.
func cloneResult(r Result) Result { r.Body = append(json.RawMessage(nil), r.Body...); return r }
// unavailable constructs a public 503 storage error without dependency details.
func unavailable() error {
	return domain.Fail(503, "DEPENDENCY_UNAVAILABLE", "Financial storage is temporarily unavailable")
}
// validateOwner returns FORBIDDEN unless owner is a UUID-shaped identity.
func validateOwner(owner string) error {
	if !domain.ValidID(owner) {
		return domain.Fail(403, "FORBIDDEN", "Invalid owner identity")
	}
	return nil
}
// Read returns a detached owner state, defaulting to a new state when absent.
// Invalid owners return FORBIDDEN; canceled contexts and serialization failures
// return DEPENDENCY_UNAVAILABLE.
func (m *Memory) Read(ctx context.Context, owner string) (*domain.State, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, unavailable()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneState(m.states[owner])
}
// Execute serializes commands and commits detached state and response copies together.
// key is an owner-scoped UUID and hash identifies the request. A matching retry
// returns the stored result with true without calling fn; new commits return false.
// Callback errors propagate without committing or consuming the key. Invalid owners,
// invalid keys, and changed retry hashes return FORBIDDEN, INVALID_IDEMPOTENCY_KEY,
// and IDEMPOTENCY_CONFLICT respectively; cancellation and serialization failures
// become DEPENDENCY_UNAVAILABLE. The callback is responsible for domain validation.
func (m *Memory) Execute(ctx context.Context, owner, key, hash string, fn func(*domain.State) (Result, error)) (Result, bool, error) {
	if err := validateOwner(owner); err != nil {
		return Result{}, false, err
	}
	if !domain.ValidID(key) {
		return Result{}, false, domain.Fail(400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a UUID")
	}
	if ctx.Err() != nil {
		return Result{}, false, unavailable()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.retries[owner][key]; ok {
		if record.Hash != hash {
			return Result{}, false, domain.Fail(409, "IDEMPOTENCY_CONFLICT", "Idempotency key was already used for another request")
		}
		return cloneResult(record.Result), true, nil
	}
	state, err := cloneState(m.states[owner])
	if err != nil {
		return Result{}, false, err
	}
	result, err := fn(state)
	if err != nil {
		return Result{}, false, err
	}
	if ctx.Err() != nil {
		return Result{}, false, unavailable()
	}
	// Serialize before commit, as PostgreSQL does; this also detaches retained callback references.
	committed, err := cloneState(state)
	if err != nil {
		return Result{}, false, err
	}
	m.states[owner] = committed
	if m.retries[owner] == nil {
		m.retries[owner] = make(map[string]retryRecord)
	}
	m.retries[owner][key] = retryRecord{hash, cloneResult(result)}
	return cloneResult(result), false, nil
}
// Ping returns DEPENDENCY_UNAVAILABLE if the context is canceled, otherwise nil.
func (m *Memory) Ping(ctx context.Context) error {
	if ctx.Err() != nil {
		return unavailable()
	}
	return nil
}
// Close is a no-op; the in-memory store has no external resources to release.
func (m *Memory) Close() {}
