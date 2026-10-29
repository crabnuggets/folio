package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

// Run the same commands through JSON-backed Memory and PostgreSQL. A new pool
// below separately proves state and exact retry bytes survive process boundaries.
func checkMonthlySheetStore(t *testing.T, db Store) (string, string, Result) {
	t.Helper()
	ctx := context.Background()
	owner := domain.NewID()
	amount, usual, repeat := domain.Money(250), domain.Money(300), true
	_, _, err := db.Execute(ctx, owner, domain.NewID(), "monthly-add", func(st *domain.State) (Result, error) {
		sheet, err := st.AddMonthlyItem("2026-10", domain.MonthlyItemInput{SectionID: "00000000-0000-4000-8000-000000000004", Name: "Synthetic card", AmountMinor: &amount, RepeatMonthly: &repeat, UsualAmountMinor: &usual}, st.MonthlySheet.Version)
		if err != nil {
			return Result{}, err
		}
		return JSON(200, sheet, sheet.Version)
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for key := range state.MonthlySheet.Items {
		id = key
	}
	if id == "" || !state.MonthlySheet.CurrencyLocked {
		t.Fatal("monthly identity or currency lock lost in store")
	}
	_, _, err = db.Execute(ctx, owner, domain.NewID(), "monthly-open", func(st *domain.State) (Result, error) {
		sheet, err := st.OpenMonthlySheet("2026-11")
		if err != nil {
			return Result{}, err
		}
		return JSON(200, sheet, sheet.Version)
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if state.MonthlySheet.Rows["2026-11"][id].AmountMinor != nil || !state.MonthlySheet.Items[id].Preferences[0].RepeatMonthly {
		t.Fatal("serialization posted amount or lost recurrence")
	}
	key := domain.NewID()
	result, _, err := db.Execute(ctx, owner, key, "monthly-remove", func(st *domain.State) (Result, error) {
		sheet, err := st.RemoveMonthlyItem("2026-10", id, true, st.MonthlySheet.Version)
		if err != nil {
			return Result{}, err
		}
		return JSON(200, sheet, sheet.Version)
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !state.MonthlySheet.Rows["2026-10"][id].Removed || state.MonthlySheet.Undo == nil || *state.MonthlySheet.Undo.Before.Rows["2026-10"][id].AmountMinor != amount {
		t.Fatal("removed amount/Undo snapshot did not survive store round trip")
	}
	// Detached reads cannot change a retained cell or recurrence preference.
	*state.MonthlySheet.Items[id].Preferences[0].UsualAmountMinor = 999
	state.MonthlySheet.Rows["2026-10"] = nil
	retained, err := db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if *retained.MonthlySheet.Items[id].Preferences[0].UsualAmountMinor != usual || retained.MonthlySheet.Rows["2026-10"] == nil {
		t.Fatal("read leaked monthly pointers")
	}
	_, _, err = db.Execute(ctx, owner, domain.NewID(), "monthly-rollback", func(st *domain.State) (Result, error) {
		st.MonthlySheet.Items = nil
		st.MonthlySheet.Version++
		return Result{}, domain.Invalid("Rollback monthly state")
	})
	if err == nil {
		t.Fatal("expected failed monthly command")
	}
	retained, err = db.Read(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(retained.MonthlySheet.Items) != 1 {
		t.Fatal("failed command committed monthly state")
	}
	return owner, key, result
}

func TestMemoryMonthlySheetSerialization(t *testing.T) { checkMonthlySheetStore(t, NewMemory()) }

func TestPostgresMonthlySheetRestart(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is unset; monthly-sheet restart needs PostgreSQL")
	}
	first, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	owner, key, result := checkMonthlySheetStore(t, first)
	first.Close()
	second, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	replayed, replay, err := second.Execute(context.Background(), owner, key, "monthly-remove", func(*domain.State) (Result, error) {
		t.Fatal("retry executed again after restart")
		return Result{}, nil
	})
	if err != nil || !replay || string(replayed.Body) != string(result.Body) || replayed.ETag != result.ETag {
		t.Fatalf("monthly retry not durable: %v %v", replay, err)
	}
	state, err := second.Read(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if state.MonthlySheet.Undo == nil {
		t.Fatal("restart lost Undo")
	}
	undo := state.MonthlySheet.Undo
	_, _, err = second.Execute(context.Background(), owner, domain.NewID(), "monthly-undo-after-restart", func(st *domain.State) (Result, error) {
		sheet, err := st.UndoMonthlySheet("2026-10", domain.MonthlyUndoInput{UndoToken: undo.Token}, st.MonthlySheet.Version)
		if err != nil {
			return Result{}, err
		}
		if sheet.MoneyOutMinor != 250 {
			t.Fatal("Undo after restart lost amount")
		}
		return JSON(200, sheet, sheet.Version)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegacyAggregateGetsEmptyMonthlySheet(t *testing.T) {
	old := domain.NewState()
	old.Settings.Version = 7
	encoded, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "monthly_sheet")
	encoded, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	loaded := domain.NewState()
	if err := json.Unmarshal(encoded, loaded); err != nil {
		t.Fatal(err)
	}
	sheet, err := loaded.GetMonthlySheet("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Version != 7 || len(sheet.Sections) != 5 || sheet.MoneyOutMinor != 0 || len(loaded.MonthlySheet.Items) != 0 {
		t.Fatal("legacy JSON did not preserve state and seed empty monthly sections")
	}
}
