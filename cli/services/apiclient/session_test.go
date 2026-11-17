package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A server that takes the request and never answers ends the call after Timeout, or at the deadline Long gives,
// with ErrTimeout.
func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	timeout, transfer := Timeout, Transfer
	Timeout, Transfer = 50*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { Timeout, Transfer = timeout, transfer })
	client, err := Session{URL: server.URL, Token: "token"}.Client()
	if err != nil {
		t.Fatal(err)
	}

	long, cancel := Long(context.Background())
	defer cancel()
	for name, ctx := range map[string]context.Context{"default": context.Background(), "long": long} {
		start := time.Now()
		_, err := client.GetApiTags(ctx)
		if !errors.Is(err, ErrTimeout) || time.Since(start) > 2*time.Second {
			t.Errorf("%s: error %v after %s, want ErrTimeout quickly", name, err, time.Since(start))
		}
	}
}
