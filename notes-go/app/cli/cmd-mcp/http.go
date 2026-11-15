package cmdmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createapitoken"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Handler serves the tools over streamable HTTP to an agent that cannot run notes mcp where it runs (the Claude app
// calls from Anthropic's cloud): each request acts for the account of its API token, "Authorization: Bearer notes_…",
// the tools calling api in process with it. Without a valid API token, 401: a session token is refused too, only an
// API token can be revoked alone and is kept off the account's routes.
func Handler(api http.Handler, version string) http.Handler {
	options := &mcp.StreamableHTTPOptions{
		// No session kept between requests: each one carries its token, and a restart loses nothing.
		Stateless:    true,
		JSONResponse: true,
		// The token authenticates every request, so a DNS rebinding page gains nothing; and a tunnel or proxy on this
		// machine (Tailscale Funnel, cloudflared) forwards from the loopback with the public Host, which this would refuse.
		DisableLocalhostProtection: true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// localhost: the API's guard takes this Host for this machine.
		client, err := apiclient.NewClientWithResponses("http://localhost", apiclient.WithHTTPClient(inProcess{api}),
			apiclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
				req.Header.Set("Authorization", "Bearer "+token)
				return nil
			}))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !strings.HasPrefix(token, createapitoken.Prefix) || !valid(r.Context(), client) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "an API token of the account is required: Authorization: Bearer notes_…", http.StatusUnauthorized)
			return
		}

		server := mcp.NewServer(&mcp.Implementation{Name: "notes", Title: "Notes", Version: version}, nil)
		AddTools(server, client)
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, options).ServeHTTP(w, r)
	})
}

// valid asks the API whether the token is known and unexpired, before any tool: the client learns it at connection.
func valid(ctx context.Context, client *apiclient.ClientWithResponses) bool {
	response, err := client.GetApiMe(ctx)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// inProcess runs the API's handler for the client's requests, without a round trip through the network.
type inProcess struct {
	api http.Handler
}

func (p inProcess) Do(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	p.api.ServeHTTP(w, r)
	return w.Result(), nil
}
