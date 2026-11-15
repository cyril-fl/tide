// Package oauth makes the MCP address an OAuth 2.1 authorization server, as the MCP specification describes, for the
// clients that take no fixed token (ChatGPT) or prefer to sign in (Claude): the metadata, an open dynamic client
// registration, the consent page and the token endpoint. A connection is a row of api_tokens: listed, revoked and
// read-only like an API token.
package oauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createapitoken"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createoauthtoken"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/refreshoauthtoken"
	"github.com/uptrace/bun"
)

const (
	// maxBody limits the bodies of the registration and the token: a few URLs and tokens.
	maxBody = 64 << 10
	// maxRedirects and maxRedirectLength bound a client_id, which carries them.
	maxRedirects      = 10
	maxRedirectLength = 512
	// defaultName names a client that gives no client_name.
	defaultName = "MCP client"
)

// Routes adds the OAuth endpoints to mux, beside /mcp.
func Routes(mux *http.ServeMux, db bun.IDB) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", resource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", resource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", server)
	mux.HandleFunc("POST /register", register)
	for _, path := range []string{"/.well-known/", "/register", "/token"} {
		mux.HandleFunc("OPTIONS "+path, preflight)
	}
	mux.Handle("GET /authorize", authorize(db))
	mux.Handle("POST /authorize", authorize(db))
	mux.Handle("POST /token", token(db))
}

// Issuer is the public URL of the MCP address, as the client reached it: the tunnel or proxy in front gives HTTPS and
// keeps the Host. Plain HTTP on the loopback only, for a test on this machine.
func Issuer(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && loopback(hostname(r.Host)) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// Challenge is the WWW-Authenticate of a 401 on /mcp: where a client finds how to sign in (RFC 9728).
func Challenge(r *http.Request) string {
	return `Bearer resource_metadata="` + Issuer(r) + `/.well-known/oauth-protected-resource"`
}

// resource is the protected resource metadata (RFC 9728): /mcp, signed in by this same address.
func resource(w http.ResponseWriter, r *http.Request) {
	issuer := Issuer(r)
	send(w, http.StatusOK, map[string]any{
		"resource":                 issuer + "/mcp",
		"resource_name":            "Notes",
		"authorization_servers":    []string{issuer},
		"bearer_methods_supported": []string{"header"},
	})
}

// server is the authorization server metadata (RFC 8414). Public clients only, with PKCE; the iss of RFC 9207 lets
// ChatGPT use its fixed redirect URI.
func server(w http.ResponseWriter, r *http.Request) {
	issuer := Issuer(r)
	send(w, http.StatusOK, map[string]any{
		"issuer":                                         issuer,
		"authorization_endpoint":                         issuer + "/authorize",
		"token_endpoint":                                 issuer + "/token",
		"registration_endpoint":                          issuer + "/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"authorization_response_iss_parameter_supported": true,
	})
}

// client is what a client_id carries: the registration keeps nothing in the base, so an open registration cannot fill
// it. Anyone can make one, as anyone can register: the consent page shows the name and the host it goes back to.
type client struct {
	Name         string   `json:"n"`
	RedirectURIs []string `json:"r"`
}

// register is the dynamic client registration (RFC 7591), open to any client: an https redirect URI, or http on the
// loopback (a client on this machine, Claude Code). Every client is public, PKCE stands for its secret.
func register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "invalid_client_metadata", "the body must be the JSON of a client")
		return
	}
	if len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > maxRedirects {
		fail(w, http.StatusBadRequest, "invalid_redirect_uri", "give 1 to 10 redirect_uris")
		return
	}
	for _, uri := range input.RedirectURIs {
		if !allowed(uri) {
			fail(w, http.StatusBadRequest, "invalid_redirect_uri", "a redirect URI must be https, or http on localhost or 127.0.0.1, without a fragment: "+uri)
			return
		}
	}
	c := client{Name: clientName(input.ClientName), RedirectURIs: input.RedirectURIs}
	id, err := json.Marshal(c)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	send(w, http.StatusCreated, map[string]any{
		"client_id":                  base64.RawURLEncoding.EncodeToString(id),
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// token is the token endpoint: a code for a new connection, or a refresh token for new tokens. A client secret sent
// anyway is ignored: every client is public.
func token(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			fail(w, http.StatusBadRequest, "invalid_request", "the body must be a form")
			return
		}
		clientID := r.PostForm.Get("client_id")
		if user, _, ok := r.BasicAuth(); ok && clientID == "" {
			clientID, _ = url.QueryUnescape(user)
		}
		c, ok := decodeClient(clientID)
		if !ok {
			fail(w, http.StatusUnauthorized, "invalid_client", "unknown client_id: register at /register")
			return
		}

		var result createoauthtoken.Result
		var err error
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			process := createoauthtoken.Process(r.Context(), db, createoauthtoken.Input{
				Code: r.PostForm.Get("code"), ClientID: clientID, ClientName: c.Name,
				RedirectURI: r.PostForm.Get("redirect_uri"), CodeVerifier: r.PostForm.Get("code_verifier"),
			})
			process.Run()
			result, err = process.GetResult(), process.GetErr()
		case "refresh_token":
			process := refreshoauthtoken.Process(r.Context(), db, refreshoauthtoken.Input{RefreshToken: r.PostForm.Get("refresh_token"), ClientID: clientID})
			process.Run()
			result, err = process.GetResult(), process.GetErr()
		default:
			fail(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
			return
		}
		switch {
		case errors.Is(err, createoauthtoken.ErrInvalidGrant):
			fail(w, http.StatusBadRequest, "invalid_grant", err.Error())
		case err != nil:
			log.Printf("oauth token: %v", err)
			fail(w, http.StatusInternalServerError, "server_error", "the server failed, try again")
		default:
			send(w, http.StatusOK, result)
		}
	}
}

// preflight lets a client in a browser (an MCP inspector) call the metadata, the registration and the token endpoint.
func preflight(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version")
	w.WriteHeader(http.StatusNoContent)
}

// Helpers

// decodeClient reads a client_id, and checks it again: it comes from the client.
func decodeClient(id string) (client, bool) {
	var c client
	data, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || json.Unmarshal(data, &c) != nil || len(c.RedirectURIs) == 0 || len(c.RedirectURIs) > maxRedirects {
		return client{}, false
	}
	for _, uri := range c.RedirectURIs {
		if !allowed(uri) {
			return client{}, false
		}
	}
	c.Name = clientName(c.Name)
	return c, true
}

// clientName is name trimmed and cut to the length of a token's name, defaultName when empty.
func clientName(name string) string {
	name = strings.TrimSpace(name)
	for utf8.RuneCountInString(name) > createapitoken.MaxName {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		return defaultName
	}
	return name
}

// allowed tells a redirect URI a client may register: https anywhere, http on the loopback only, no fragment.
func allowed(uri string) bool {
	if len(uri) > maxRedirectLength {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && loopback(u.Hostname()))
}

// redirectOf is the redirect URI of c that asked matches, "" when none: the same string, or on the loopback the same
// but its port, which a native client picks at each sign-in (RFC 8252).
func redirectOf(c client, asked string) string {
	a, err := url.Parse(asked)
	if err != nil {
		return ""
	}
	for _, uri := range c.RedirectURIs {
		if uri == asked {
			return asked
		}
		u, err := url.Parse(uri)
		if err == nil && u.Scheme == "http" && a.Scheme == "http" && loopback(u.Hostname()) &&
			u.Hostname() == a.Hostname() && u.Path == a.Path && u.RawQuery == a.RawQuery && a.Fragment == "" && a.User == nil {
			return asked
		}
	}
	return ""
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostname is host without its port.
func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// send writes an OAuth answer: JSON, never cached (it may hold tokens).
func send(w http.ResponseWriter, status int, body any) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// fail writes an OAuth error (RFC 6749 section 5.2).
func fail(w http.ResponseWriter, status int, code, description string) {
	send(w, status, map[string]string{"error": code, "error_description": description})
}
