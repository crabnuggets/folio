package store

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

func checkStore(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	owner := domain.NewID()
	other := domain.NewID()
	key := domain.NewID()
	var executions atomic.Int32
	command := func(state *domain.State) (Result, error) {
		executions.Add(1)
		state.Settings.Version++
		state.Audit("test", owner, "")
		return JSON(201, state.Settings, state.Settings.Version)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			result, _, err := s.Execute(ctx, owner, key, "same-request", command)
			if err != nil {
				t.Errorf("concurrent execute: %v", err)
				return
			}
			if result.Status != 201 || result.ETag != `"2"` {
				t.Errorf("wrong replay: %+v", result)
			}
		})
	}
	wg.Wait()
	if executions.Load() != 1 {
		t.Fatalf("executed %d times", executions.Load())
	}
	state, err := s.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if state.Settings.Version != 2 || len(state.AuditEvents) != 1 {
		t.Fatalf("duplicate or lost commit: %+v", state.Settings)
	}
	state.Settings.Currency = "USD"
	reread, _ := s.Read(ctx, owner)
	if reread.Settings.Currency != "SGD" {
		t.Fatal("read leaked mutable state")
	}
	isolated, err := s.Read(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if isolated.Settings.Version != 1 || len(isolated.AuditEvents) != 0 {
		t.Fatal("owner data leaked")
	}
	if _, _, err := s.Execute(ctx, owner, key, "changed-request", command); err == nil {
		t.Fatal("reused key accepted with changed request")
	}
	failKey := domain.NewID()
	_, _, err = s.Execute(ctx, owner, failKey, "fails", func(s *domain.State) (Result, error) {
		s.Settings.Version = 100
		s.Audit("should_rollback", owner, "")
		return Result{}, domain.Invalid("Rejected after tentative mutation")
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	state, _ = s.Read(ctx, owner)
	if state.Settings.Version != 2 || len(state.AuditEvents) != 1 {
		t.Fatal("failed transaction committed")
	}
	result, replay, err := s.Execute(ctx, owner, failKey, "fixed", command)
	if err != nil || replay || result.ETag != `"3"` {
		t.Fatalf("failed command consumed key: %+v %t %v", result, replay, err)
	}
	if _, _, err := s.Execute(ctx, other, key, "same-request", command); err != nil {
		t.Fatal("key not scoped by owner", err)
	}
	if _, err = s.Read(ctx, "bad-owner"); err == nil {
		t.Fatal("invalid owner accepted")
	}
	if _, _, err = s.Execute(ctx, owner, "bad-key", "x", command); err == nil {
		t.Fatal("invalid key accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = s.Execute(cancelled, owner, domain.NewID(), "cancelled", command); err == nil {
		t.Fatal("cancelled command succeeded")
	}
}
func TestMemoryTransactionalContract(t *testing.T) { checkStore(t, NewMemory()) }
func TestPostgresTransactionalContract(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset; real PostgreSQL integration test requires local database")
	}
	s, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	checkStore(t, s)
}
func TestPostgresRetrySurvivesNewPool(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	ctx := context.Background()
	first, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	owner, key := domain.NewID(), domain.NewID()
	result, _, err := first.Execute(ctx, owner, key, "persisted", func(s *domain.State) (Result, error) { s.Settings.Version = 8; return JSON(201, s.Settings, 8) })
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.Close()
	second, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	replayed, replay, err := second.Execute(ctx, owner, key, "persisted", func(*domain.State) (Result, error) { t.Fatal("durable retry re-executed"); return Result{}, nil })
	if err != nil || !replay || string(result.Body) != string(replayed.Body) {
		t.Fatalf("retry not retained: %t %v", replay, err)
	}
	state, _ := second.Read(ctx, owner)
	if state.Settings.Version != 8 {
		t.Fatal("state not retained")
	}
}
func TestMemoryRetainedReferencesCannotMutateCommit(t *testing.T) {
	m := NewMemory()
	owner, key := domain.NewID(), domain.NewID()
	var retained *domain.State
	result, _, err := m.Execute(context.Background(), owner, key, "hash", func(s *domain.State) (Result, error) { retained = s; return JSON(201, s.Settings, 1) })
	if err != nil {
		t.Fatal(err)
	}
	retained.Settings.Version = 100
	result.Body[0] = 'x'
	if !json.Valid(m.retries[owner][key].Result.Body) {
		t.Fatal("result aliases retry bytes")
	}
	state, _ := m.Read(context.Background(), owner)
	if state.Settings.Version != 1 {
		t.Fatal("retained state mutated committed snapshot")
	}
}

func TestPostgresRowLevelOwnershipDefense(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	ctx := context.Background()
	s, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, other := domain.NewID(), domain.NewID()
	for _, id := range []string{owner, other} {
		_, _, err = s.Execute(ctx, id, domain.NewID(), "seed", func(state *domain.State) (Result, error) { return JSON(201, state.Settings, 1) })
		if err != nil {
			t.Fatal(err)
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	roleName := "folio_rls_test_" + strings.ReplaceAll(domain.NewID(), "-", "")
	// Integration database must be disposable and permit creating a transient role.
	if _, err = tx.Exec(ctx, "CREATE ROLE "+roleName+" NOLOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Skip("database role cannot create a transient RLS test role")
	}
	if _, err = tx.Exec(ctx, "GRANT USAGE ON SCHEMA folio TO "+roleName); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "GRANT SELECT ON folio.owner_state,folio.idempotency_records TO "+roleName); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE "+roleName); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('folio.owner_id',$1,true)", owner); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM folio.owner_state").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RLS exposed %d owners", count)
	}
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM folio.owner_state WHERE owner_id=$1", other).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("RLS exposed a foreign owner")
	}
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM folio.idempotency_records").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RLS exposed %d retry records", count)
	}
}
