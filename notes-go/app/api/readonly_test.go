package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/api/services/auth"
)

// A read-only API token reads the notes, and every change is refused with 403, the header telling it.
func TestReadOnlyAPIToken(t *testing.T) {
	c := newClient(t)
	var created struct {
		Token    string `json:"token"`
		APIToken struct {
			ReadOnly bool `json:"read_only"`
		} `json:"api_token"`
	}
	c.do("POST", "/api/me/tokens", `{"name":"reader","read_only":true}`, http.StatusCreated, &created)
	if !created.APIToken.ReadOnly {
		t.Fatalf("created = %+v, want read_only", created.APIToken)
	}

	send := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "localhost:8080"
		r.Header.Set("Authorization", "Bearer "+created.Token)
		w := httptest.NewRecorder()
		c.api.ServeHTTP(w, r)
		return w
	}
	for _, route := range [][3]string{
		{"GET", "/api/notes", ""},
		{"GET", "/api/me", ""},
		{"POST", "/api/notes/tags", `{"ids":[]}`},
	} {
		if w := send(route[0], route[1], route[2]); w.Code >= 400 || w.Header().Get(auth.ReadOnlyHeader) != "true" {
			t.Errorf("%s %s read-only: status = %d, header = %q; want it served, the header set: %s", route[0], route[1], w.Code, w.Header().Get(auth.ReadOnlyHeader), w.Body)
		}
	}
	for _, route := range [][3]string{
		{"POST", "/api/notes", `{"content":"# no"}`},
		{"POST", "/api/folders", `{"title":"no"}`},
		{"DELETE", "/api/trash", ""},
		{"POST", "/api/move", `{"ids":[]}`},
	} {
		if w := send(route[0], route[1], route[2]); w.Code != http.StatusForbidden {
			t.Errorf("%s %s read-only: status = %d, want 403", route[0], route[1], w.Code)
		}
	}
	if count := c.raw("/api/notes"); strings.Contains(count, "# no") {
		t.Errorf("a read-only token wrote a note: %s", count)
	}
}
