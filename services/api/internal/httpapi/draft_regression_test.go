package httpapi_test

import (
	"fmt"
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"testing"
)

func TestStatementDraftPreservesUnknownPayableAndExplicitZero(t *testing.T) {
	for _, due := range []string{"absent", "null", "zero"} {
		t.Run(due, func(t *testing.T) {
			h := newHarness(t)
			cardBody := accountInput("Card")
			cardBody["kind"] = "credit_card"
			cardBody["include_in_liquid_cash"] = false
			card := expect(t, h.request(t, "POST", "/api/v1/accounts", cardBody, domain.NewID(), ""), 201, "Account")
			statement := map[string]any{"account_id": card["id"], "currency": "SGD", "cycle_start": "2026-09-01", "cycle_end": "2026-09-30", "issue_date": "2026-10-01", "due_date": "2026-10-20", "net_spending_minor": "10000", "spending_evidence": "Synthetic"}
			if due == "null" {
				statement["amount_due_minor"] = nil
			}
			if due == "zero" {
				statement["amount_due_minor"] = "0"
			}
			draft := expect(t, h.request(t, "POST", "/api/v1/drafts", map[string]any{"source_id": "manual", "external_id": "payable-presence", "evidence": "Synthetic partial statement", "statement": statement}, domain.NewID(), ""), 201, "Draft")
			accepted := h.request(t, "POST", fmt.Sprintf("/api/v1/drafts/%s/accept", draft["id"]), map[string]any{}, domain.NewID(), `"1"`)
			if due == "zero" {
				out := expect(t, accepted, 200, "Draft")
				posted := expect(t, h.request(t, "GET", fmt.Sprintf("/api/v1/statements/%s", out["accepted_record_id"]), nil, "", ""), 200, "Statement")
				if posted["amount_due_minor"] != "0" {
					t.Fatal(posted)
				}
			} else {
				expect(t, accepted, 422, "ErrorEnvelope")
				pending := expect(t, h.request(t, "GET", fmt.Sprintf("/api/v1/drafts/%s", draft["id"]), nil, "", ""), 200, "Draft")
				if pending["state"] != "pending" {
					t.Fatal(pending)
				}
				page := expect(t, h.request(t, "GET", "/api/v1/statements", nil, "", ""), 200, "StatementPage")
				if len(page["items"].([]any)) != 0 {
					t.Fatal("unknown payable posted")
				}
			}
		})
	}
}
