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

func NewMemory() *Memory {
	return &Memory{states: make(map[string]*domain.State), retries: make(map[string]map[string]retryRecord)}
}
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
func cloneResult(r Result) Result { r.Body = append(json.RawMessage(nil), r.Body...); return r }
func unavailable() error {
	return domain.Fail(503, "DEPENDENCY_UNAVAILABLE", "Financial storage is temporarily unavailable")
}
func validateOwner(owner string) error {
	if !domain.ValidID(owner) {
		return domain.Fail(403, "FORBIDDEN", "Invalid owner identity")
	}
	return nil
}
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
func (m *Memory) Ping(ctx context.Context) error {
	if ctx.Err() != nil {
		return unavailable()
	}
	return nil
}
func (m *Memory) Close() {}
