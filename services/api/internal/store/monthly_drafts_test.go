package store

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

func checkMonthlyDraftStore(t *testing.T, db Store) (string, string, string, Result) {
	t.Helper()
	ctx := context.Background()
	owner := domain.NewID()
	input := domain.MonthlyDraftInput{Month: "2026-11", SectionID: "00000000-0000-4000-8000-000000000004", Name: "Persisted proposal", AmountMinor: 1250, SourceID: "test:store", ExternalID: "one", SheetVersion: 1}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			_, _, err := db.Execute(ctx, owner, domain.NewID(), "stage", func(st *domain.State) (Result, error) {
				d, _, err := st.CreateMonthlyDraft(input)
				if err != nil {
					return Result{}, err
				}
				return JSON(201, d, d.Version)
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	state, err := db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.MonthlyDrafts) != 1 || state.MonthlySheet.Version != 1 || len(state.MonthlySheet.Items) != 0 {
		t.Fatal("concurrent staging changed totals or duplicated proposal")
	}
	var draft domain.MonthlyDraft
	for _, d := range state.MonthlyDrafts {
		draft = d
	}
	// A tentative accept that later fails must roll back both the row and review.
	_, _, err = db.Execute(ctx, owner, domain.NewID(), "rollback", func(st *domain.State) (Result, error) {
		_, err := st.AcceptMonthlyDraft(draft.ID, 1, 1)
		if err != nil {
			return Result{}, err
		}
		return Result{}, domain.Invalid("Synthetic failure after acceptance")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	state, err = db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if state.MonthlyDrafts[draft.ID].State != "pending" || len(state.MonthlySheet.Items) != 0 {
		t.Fatal("failed acceptance committed")
	}
	key := domain.NewID()
	var original Result
	for i := range 12 {
		wg.Go(func() {
			result, _, err := db.Execute(ctx, owner, key, "accept", func(st *domain.State) (Result, error) {
				sheet, err := st.AcceptMonthlyDraft(draft.ID, 1, 1)
				if err != nil {
					return Result{}, err
				}
				return JSON(200, sheet, sheet.Version)
			})
			if err != nil {
				t.Error(err)
			}
			if i == 0 {
				original = result
			}
		})
	}
	wg.Wait()
	state, err = db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := state.GetMonthlySheet(input.Month)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.MoneyOutMinor != 1250 || len(state.MonthlySheet.Items) != 1 || state.MonthlyDrafts[draft.ID].State != "accepted" {
		t.Fatal("concurrent acceptance duplicated or lost row")
	}
	acceptedEvents := 0
	for _, event := range state.AuditEvents {
		if event.Action == "monthly_draft.accepted" {
			acceptedEvents++
		}
	}
	if acceptedEvents != 1 {
		t.Fatal("acceptance audit duplicated")
	}
	other, err := db.Read(ctx, domain.NewID())
	if err != nil {
		t.Fatal(err)
	}
	if len(other.MonthlyDrafts) != 0 {
		t.Fatal("proposal leaked to another owner")
	}
	return owner, draft.ID, key, original
}
func TestMemoryMonthlyDraftLifecycle(t *testing.T) { checkMonthlyDraftStore(t, NewMemory()) }
func TestPostgresMonthlyDraftRestartAndRetry(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	db, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	owner, draftID, key, original := checkMonthlyDraftStore(t, db)
	db.Close()
	db, err = NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	replay, replayed, err := db.Execute(context.Background(), owner, key, "accept", func(*domain.State) (Result, error) {
		t.Fatal("acceptance executed after restart")
		return Result{}, nil
	})
	if err != nil || !replayed || string(replay.Body) != string(original.Body) || replay.ETag != original.ETag {
		t.Fatal("acceptance retry bytes lost after restart", err)
	}
	state, err := db.Read(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if state.MonthlyDrafts[draftID].State != "accepted" {
		t.Fatal("review state lost after restart")
	}
}
func TestPostgresRetiredAggregateFieldsSurviveMonthlyWrite(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset")
	}
	db, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	owner := domain.NewID()
	state := domain.NewState()
	amount := domain.Money(4200)
	if _, err = state.AddMonthlyItem("2026-11", domain.MonthlyItemInput{SectionID: "00000000-0000-4000-8000-000000000001", Name: "Existing salary", AmountMinor: &amount}, 1); err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(document, &fields)
	fields["entries"] = json.RawMessage(`{"retired":{"unknown":"retained","amount_minor":123.4}}`)
	document, _ = json.Marshal(fields)
	tx, err := db.begin(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO folio.owner_state(owner_id,document) VALUES($1,$2)", owner, document); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, err = db.Execute(ctx, owner, domain.NewID(), "new-proposal", func(st *domain.State) (Result, error) {
		d, _, err := st.CreateMonthlyDraft(domain.MonthlyDraftInput{Month: "2026-11", SectionID: "00000000-0000-4000-8000-000000000004", Name: "New card", AmountMinor: 100, SourceID: "test", ExternalID: "one", SheetVersion: st.MonthlySheet.Version})
		if err != nil {
			return Result{}, err
		}
		return JSON(201, d, d.Version)
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := loaded.GetMonthlySheet("2026-11")
	if err != nil || sheet.MoneyInMinor != 4200 || sheet.MoneyOutMinor != 0 {
		t.Fatal("previous monthly amounts changed", err)
	}
	document, _ = json.Marshal(loaded)
	_ = json.Unmarshal(document, &fields)
	var retired map[string]any
	_ = json.Unmarshal(fields["entries"], &retired)
	if retired["retired"].(map[string]any)["unknown"] != "retained" {
		t.Fatal("retired data was deleted on write")
	}
}
