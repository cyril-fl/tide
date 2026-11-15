package cmdmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createapitoken"
	"github.com/cyril-fl/tide/notes-go/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bearer sends a token with every request of the MCP client, as a connector of the Claude app does.
type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

// TestHandler serves the tools over HTTP for an API token only, and acts for its account.
func TestHandler(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	api, err := url.Parse(os.Getenv(config.Env("URL")))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(Handler(httputil.NewSingleHostReverseProxy(api), "test"))
	t.Cleanup(server.Close)

	newToken := func(name string) (string, string) {
		process := createapitoken.Process(ctx, tx, createapitoken.Input{Name: name})
		process.Run()
		if process.HasErr() {
			t.Fatal(process.GetErr())
		}
		return process.GetResult().Token, process.GetResult().APIToken.ID
	}
	token, _ := newToken("phone")
	expired, expiredID := newToken("expired")
	if _, err := tx.NewUpdate().Model((*domain.APIToken)(nil)).Set("expires_at = now() - interval '1 minute'").Where("id = ?", expiredID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	revoked, revokedID := newToken("revoked")
	if _, err := tx.NewDelete().Model((*domain.APIToken)(nil)).Where("id = ?", revokedID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	for name, token := range map[string]string{
		"none": "", "session": os.Getenv(config.Env("SESSION")), "unknown": createapitoken.Prefix + "nope",
		"expired": expired, "revoked": revoked,
	} {
		request, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		response, err := (&http.Client{Transport: bearer(token)}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s token: %d, want 401", name, response.StatusCode)
		}
		if want := `Bearer resource_metadata="http://` + request.URL.Host + `/.well-known/oauth-protected-resource"`; response.Header.Get("WWW-Authenticate") != want {
			t.Errorf("%s token: WWW-Authenticate = %q, want %q", name, response.Header.Get("WWW-Authenticate"), want)
		}
	}

	transport := &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearer(token)}}
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agent.Close() })
	var created, read note
	call(t, agent, "create_note", map[string]any{"content": "# From the phone"}, &created)
	call(t, agent, "read_note", map[string]any{"note": "/From the phone"}, &read)
	if read.ID != created.ID || read.Title != "From the phone" {
		t.Errorf("read_note = %+v, want %+v", read, created)
	}
}

// A read-only token gets the reading tools only.
func TestHandlerReadOnly(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	api, err := url.Parse(os.Getenv(config.Env("URL")))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(Handler(httputil.NewSingleHostReverseProxy(api), "test"))
	t.Cleanup(server.Close)
	process := createapitoken.Process(ctx, tx, createapitoken.Input{Name: "reader", ReadOnly: true})
	process.Run()
	if process.HasErr() {
		t.Fatal(process.GetErr())
	}

	transport := &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearer(process.GetResult().Token)}}
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agent.Close() })
	tools, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("read-only token: %s is listed, it writes", tool.Name)
		}
		names[tool.Name] = true
	}
	if !names["read_note"] || !names["search_notes"] || names["create_note"] {
		t.Errorf("read-only tools = %v, want read_note and search_notes, not create_note", names)
	}
}
