package httpapi

import (
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"testing"
)

func TestRequiredFieldsAndNullsPreserveWireContract(t *testing.T) {
	for _, body := range []string{`{"alias":"Cash","kind":"cash","currency":"SGD"}`, `{"alias":"Cash","kind":"cash","currency":"SGD","active":null}`, `{"alias":"Cash","kind":"cash","currency":"SGD","active":false,"include_in_liquid_cash":null}`, `{"alias":"Cash","kind":"cash","currency":"SGD","active":false,"unknown":true}`, `null`, `[]`, `{} {}`} {
		if _, err := readJSON[domain.AccountInput]([]byte(body)); err == nil {
			t.Fatalf("accepted malformed account: %s", body)
		}
	}
	if _, err := readJSON[domain.AccountInput]([]byte(`{"alias":"Cash","kind":"cash","currency":"SGD","active":false}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSON[domain.StatementInput]([]byte(`{"account_id":"a","currency":"SGD","cycle_start":"2026-09-01","cycle_end":"2026-09-30","issue_date":"2026-10-01","due_date":"2026-10-20"}`)); err == nil {
		t.Fatal("omitted due amount became zero")
	}
	if _, err := readJSON[domain.DraftInput]([]byte(`{"source_id":"manual","external_id":"one","evidence":"Synthetic","entry":{"kind":"income"}}`)); err != nil {
		t.Fatalf("partial candidate must remain stageable: %v", err)
	}
}
