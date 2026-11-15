// Package auth lets through the requests of a logged-in account only, and tells the routes which one.
package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createapitoken"
	"github.com/cyril-fl/tide/core/services/process/createsession"
	"github.com/cyril-fl/tide/core/services/process/readapitoken"
	"github.com/cyril-fl/tide/core/services/process/readsession"
	"github.com/uptrace/bun"
)

// Cookie holds the session token of the web app: HttpOnly, page scripts never see it.
var Cookie = config.Current.Name + "_session"

// ReadOnlyHeader is set to "true" on the answers to a read-only API token.
const ReadOnlyHeader = "Notes-Read-Only"

// readRoutes are the routes that read with a POST, for a body too long for a query: a read-only token may call them.
var readRoutes = map[string]bool{"/api/notes/tags": true, "/api/searches": true}

// Public routes answer without a session.
var public = map[string]bool{"POST /api/login": true, "POST /api/signup": true, "GET /api/add-user": true}

// New runs next with the account of the request's session in its context: the cookie, or a
// "Authorization: Bearer <token>" header for scripts, a session token or an API token. Without a valid one, 401.
func New(db bun.IDB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public[r.Method+" "+r.URL.Path] || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		token := Token(r)
		if strings.HasPrefix(token, createapitoken.Prefix) {
			apiToken(db, next, w, r, token)
			return
		}
		session := readsession.Process(r.Context(), db, token)
		session.Run()
		if session.HasErr() {
			respond.Error(w, session.GetErr())
			return
		}
		// readsession started the session's lifetime again: the cookie follows.
		SetCookie(w, token, time.Now().Add(createsession.Lifetime))
		next.ServeHTTP(w, r.WithContext(domain.WithUser(r.Context(), session.GetResult().ID)))
	})
}

// apiToken runs next for the account of an API token, without a cookie: a script has no use of one. The token
// works on the notes and refuses the account's own routes with 403.
func apiToken(db bun.IDB, next http.Handler, w http.ResponseWriter, r *http.Request, token string) {
	process := readapitoken.Process(r.Context(), db, token)
	process.Run()
	if process.HasErr() {
		respond.Error(w, process.GetErr())
		return
	}
	if account(r) {
		respond.JSON(w, http.StatusForbidden, respond.Failure{Error: "an API token cannot manage the account: log in"})
		return
	}
	if process.GetReadOnly() {
		// Every answer tells it: the MCP over HTTP leaves out its writing tools.
		w.Header().Set(ReadOnlyHeader, "true")
		if !reads(r) {
			respond.JSON(w, http.StatusForbidden, respond.Failure{Error: "this token is read-only: it cannot change anything"})
			return
		}
	}
	next.ServeHTTP(w, r.WithContext(domain.WithUser(r.Context(), process.GetResult().ID)))
}

// account tells the routes that manage the account, refused to an API token: its tokens, password, settings, data,
// logout, and the admin's. Reading the account and its stats stays allowed. The mux has not matched the route yet,
// so this goes by path; the mux redirects an unclean path (//, ..) instead of serving it.
func account(r *http.Request) bool {
	path := r.URL.Path
	if r.Method == http.MethodGet && (path == "/api/me" || path == "/api/me/stats") {
		return false
	}
	for _, prefix := range []string{"/api/me", "/api/logout", "/api/users", "/api/invites"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// reads tells a request that changes nothing: a GET, or a POST of readRoutes.
func reads(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead || (r.Method == http.MethodPost && readRoutes[r.URL.Path])
}

// Token is the request's session token, "" without one.
func Token(r *http.Request) string {
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return bearer
	}
	if cookie, err := r.Cookie(Cookie); err == nil {
		return cookie.Value
	}
	// TODO: the cookie of before Tide still logs in, remove it createsession.Lifetime after the server runs Tide.
	if cookie, err := r.Cookie("notes_session"); err == nil {
		return cookie.Value
	}
	return ""
}

// SetCookie gives the browser the session token until expires; a zero time deletes it.
// Secure works on http://localhost too: browsers count it as a secure context.
func SetCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{Name: Cookie, Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, Expires: expires}
	if expires.IsZero() {
		cookie.MaxAge = -1
	}
	http.SetCookie(w, cookie)
}
