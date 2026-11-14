package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A path asks the server, with the kind and the token; a uuid is kept as is, without a request; a 404 gives the
// API's message.
func TestResolve(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/api/resolve" || query.Get("kind") != "note" || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("request %s, Authorization %q", r.URL, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("path") == "/Projects/Ideas" {
			w.Write([]byte(`{"id":"the-uuid"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()
	client, err := Session{URL: server.URL, Token: "token"}.Client()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if id, err := Resolve(ctx, client, NOTE, "/Projects/Ideas"); err != nil || id != "the-uuid" {
		t.Errorf("path: %q, %v; want the-uuid", id, err)
	}
	if id, err := Resolve(ctx, client, NOTE, "a-uuid"); err != nil || id != "a-uuid" || requests != 1 {
		t.Errorf("uuid: %q, %v, %d requests; want it kept without a request", id, err, requests)
	}
	if _, err := Resolve(ctx, client, NOTE, "/Nowhere"); err == nil || err.Error() != "not found" {
		t.Errorf("no item: err = %v, want the API's message", err)
	}
}
