package httpapi

import (
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"testing"
)

func TestMonthlyDraftReviewRequiresCanonicalExplicitMoney(t *testing.T) {
	for _, body := range []string{
		`{"section_id":"section","name":"Card","note":""}`,
		`{"section_id":"section","name":"Card","amount_minor":null,"note":""}`,
		`{"section_id":"section","name":"Card","amount_minor":0,"note":""}`,
		`{"section_id":"section","name":"Card","amount_minor":"01","note":""}`,
		`{"section_id":"section","name":"Card","amount_minor":"9223372036854775808","note":""}`,
		`{"section_id":"section","name":"Card","amount_minor":"0","note":"","owner_id":"someone"}`,
	} {
		if _, err := readJSON[domain.MonthlyDraftEdit]([]byte(body)); err == nil {
			t.Fatalf("accepted ambiguous review input: %s", body)
		}
	}
	if _, err := readJSON[domain.MonthlyDraftEdit]([]byte(`{"section_id":"section","name":"Card","amount_minor":"0","note":""}`)); err != nil {
		t.Fatal("explicit zero must stay valid", err)
	}
	if _, err := readJSON[domain.MonthlyDraftAccept]([]byte(`{}`)); err == nil {
		t.Fatal("acceptance must include sheet version")
	}
}
