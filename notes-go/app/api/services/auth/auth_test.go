package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The cookie of before the rename to Tide still logs in; the new one wins when both are sent.
func TestTokenOldCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.AddCookie(&http.Cookie{Name: "notes_session", Value: "old"})
	if token := Token(r); token != "old" {
		t.Errorf("notes_session: Token = %q, want old", token)
	}
	r.AddCookie(&http.Cookie{Name: Cookie, Value: "new"})
	if token := Token(r); token != "new" || Cookie != "tide_session" {
		t.Errorf("both: Token = %q, Cookie = %q; want new, tide_session", token, Cookie)
	}
}
