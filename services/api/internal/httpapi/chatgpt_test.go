package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
)

func TestChatGPTConnectionUnavailableAndRequiresOwnerAuthentication(t *testing.T) {
	h := newHarness(t)
	status := expect(t, h.request(t, "GET", "/api/v1/chatgpt/connection", nil, "", ""), 200, "ChatGPTConnection")
	if status["available"] != false || status["connected"] != false || status["reason"] == "" {
		t.Fatal(status)
	}
	assertError(t, h.request(t, "POST", "/api/v1/chatgpt/connect", map[string]any{}, "", ""), 503, "CHATGPT_UNAVAILABLE")
	response, err := h.client.Get(h.server.URL + "/api/v1/chatgpt/connection")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("anonymous provider status was accessible")
	}
	response, err = h.client.Get(h.server.URL + "/auth/chatgpt/callback?code=synthetic&state=synthetic")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("callback without browser binding was accepted")
	}
}

func TestExportOmitsPrivateChatGPTStateWithoutDeletingConnection(t *testing.T) {
	db := store.NewMemory()
	ctx := context.Background()
	_, _, err := db.Execute(ctx, ownerID, domain.NewID(), "seed-provider", func(st *domain.State) (store.Result, error) {
		st.ChatGPTConnection = &domain.ChatGPTConnection{HostID: "private-host", ClientID: "private-client", Credentials: "encrypted-private-credentials"}
		return store.JSON(200, map[string]bool{"saved": true}, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(db, ownerID, func(next http.Handler) http.Handler { return next }))
	defer server.Close()
	h := &harness{server: server, client: server.Client()}
	response := h.request(t, http.MethodGet, "/api/v1/exports", nil, "", "")
	expect(t, response, 200, "Export")
	for _, private := range []string{"chatgpt_connection", "private-host", "private-client", "encrypted-private-credentials"} {
		if strings.Contains(string(response.body), private) {
			t.Fatal("export contains private provider state", private)
		}
	}
	saved, err := db.Read(ctx, ownerID)
	if err != nil || saved.ChatGPTConnection == nil || saved.ChatGPTConnection.Credentials != "encrypted-private-credentials" {
		t.Fatal("export deleted persisted connection", err)
	}
}
