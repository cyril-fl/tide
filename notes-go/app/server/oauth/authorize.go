package oauth

import (
	"errors"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/api/services/throttle"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createoauthcode"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createsession"
	"github.com/cyril-fl/tide/notes-go/config"
	"github.com/uptrace/bun"
)

// request is what a client asks /authorize, carried through the page's form.
type request struct {
	ClientID      string
	RedirectURI   string
	State         string
	CodeChallenge string
	Client        client
}

// page is what the consent page shows.
type page struct {
	request
	Text     texts
	Host     string // where the code goes: the client is whoever holds this host
	Loopback bool
	Name     string
	ReadOnly bool
	Error    string
}

// authorize is the consent page: the account signs in with its name and password, and allows the client, read-only if
// it ticks so. Its wrong passwords count with the login's (throttle.Passwords), by address: it faces the Internet.
// A client_id or redirect_uri that does not hold answers the page alone, never a redirect: it could go anywhere.
func authorize(db bun.IDB) http.HandlerFunc {
	failed := throttle.Passwords(db)
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		text := language(r)
		params := r.Form
		c, ok := decodeClient(params.Get("client_id"))
		redirect := redirectOf(c, params.Get("redirect_uri"))
		if !ok || redirect == "" {
			show(w, http.StatusBadRequest, page{Text: text, Error: text.BadClient})
			return
		}
		req := request{ClientID: params.Get("client_id"), RedirectURI: redirect, State: params.Get("state"), CodeChallenge: params.Get("code_challenge"), Client: c}
		switch {
		case params.Get("response_type") != "code":
			back(w, r, req, url.Values{"error": {"unsupported_response_type"}})
			return
		case req.CodeChallenge == "" || params.Get("code_challenge_method") != "S256":
			back(w, r, req, url.Values{"error": {"invalid_request"}, "error_description": {"PKCE with code_challenge_method=S256 is required"}})
			return
		}

		target, _ := url.Parse(redirect)
		shown := page{request: req, Text: text, Host: target.Host, Loopback: loopback(target.Hostname())}
		if r.Method == http.MethodGet {
			show(w, http.StatusOK, shown)
			return
		}
		if r.PostForm.Get("decision") != "allow" {
			back(w, r, req, url.Values{"error": {"access_denied"}})
			return
		}

		shown.Name, shown.ReadOnly = r.PostForm.Get("name"), r.PostForm.Get("read_only") == "on"
		address, _, _ := net.SplitHostPort(r.RemoteAddr)
		key := "address " + address
		wait, err := failed.Try(r.Context(), key)
		if err != nil {
			log.Printf("oauth authorize: %v", err)
			shown.Error = text.Failed
			show(w, http.StatusInternalServerError, shown)
			return
		}
		if wait > 0 {
			w.Header().Set("Retry-After", throttle.RetryAfter(wait))
			shown.Error = strings.ReplaceAll(text.TooMany, "{wait}", wait.Round(time.Second).String())
			show(w, http.StatusTooManyRequests, shown)
			return
		}

		process := createoauthcode.Process(r.Context(), db, createoauthcode.Input{
			Name: shown.Name, Password: r.PostForm.Get("password"),
			ClientID: req.ClientID, RedirectURI: redirect, CodeChallenge: req.CodeChallenge, ReadOnly: shown.ReadOnly,
		})
		process.Run()
		if process.HasErr() {
			if errors.Is(process.GetErr(), createsession.ErrBadCredentials) {
				shown.Error = text.BadCredentials
				show(w, http.StatusUnauthorized, shown)
				return
			}
			failed.Forgive(r.Context(), key)
			log.Printf("oauth authorize: %v", process.GetErr())
			shown.Error = text.Failed
			show(w, http.StatusInternalServerError, shown)
			return
		}
		failed.Reset(r.Context(), key)
		back(w, r, req, url.Values{"code": {process.GetResult()}})
	}
}

// back sends the browser to the client with values, its state and the issuer (RFC 9207): success or error alike.
func back(w http.ResponseWriter, r *http.Request, req request, values url.Values) {
	target, _ := url.Parse(req.RedirectURI)
	query := target.Query()
	for key, value := range values {
		query[key] = value
	}
	if req.State != "" {
		query.Set("state", req.State)
	}
	query.Set("iss", Issuer(r))
	target.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// show writes the page, which no other site can frame: a hidden frame would make a click allow a client.
func show(w http.ResponseWriter, status int, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	if err := consent.Execute(w, p); err != nil {
		log.Printf("oauth page: %v", err)
	}
}

// texts are the words of the page, in English or French.
type texts struct {
	Lang, Title, Wants, Back, Local, Name, Password, ReadOnly, ReadOnlyHelp, Allow, Deny string
	BadClient, BadCredentials, TooMany, Failed                                           string
}

var english = texts{
	Lang: "en", Title: "Connect to " + config.Title(),
	Wants:        "wants to read and change all your notes, as you can.",
	Back:         "Once you allow it, you go back to",
	Local:        "a program on this device: allow it only if you started the sign-in here.",
	Name:         "Account name",
	Password:     "Password",
	ReadOnly:     "Read only",
	ReadOnlyHelp: "It reads the notes and changes nothing.",
	Allow:        "Allow", Deny: "Deny",
	BadClient:      "This link is not a valid sign-in request: start again from the app you are connecting.",
	BadCredentials: "Wrong account name or password.",
	TooMany:        "Too many wrong passwords: try again in {wait}.",
	Failed:         "The server failed: try again.",
}

var french = texts{
	Lang: "fr", Title: "Connexion à " + config.Title(),
	Wants:        "veut lire et modifier toutes vos notes, comme vous.",
	Back:         "Une fois autorisé, vous revenez sur",
	Local:        "un programme de cet appareil : n'autorisez que si vous avez lancé la connexion ici.",
	Name:         "Nom du compte",
	Password:     "Mot de passe",
	ReadOnly:     "Lecture seule",
	ReadOnlyHelp: "Il lit les notes et ne change rien.",
	Allow:        "Autoriser", Deny: "Refuser",
	BadClient:      "Ce lien n'est pas une demande de connexion valide : recommencez depuis l'application à connecter.",
	BadCredentials: "Nom de compte ou mot de passe incorrect.",
	TooMany:        "Trop de mots de passe incorrects : réessayez dans {wait}.",
	Failed:         "Le serveur a échoué : réessayez.",
}

// language is French when the browser prefers it, English otherwise.
func language(r *http.Request) texts {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept-Language"))), "fr") {
		return french
	}
	return english
}

var consent = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="{{.Text.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{{.Text.Title}}</title>
<style>
:root { color-scheme: light dark; --bg: #f8fafc; --card: #fff; --text: #0f172a; --muted: #64748b; --border: #e2e8f0; --accent: #16a34a; --error: #dc2626; }
@media (prefers-color-scheme: dark) { :root { --bg: #0f172a; --card: #1e293b; --text: #f1f5f9; --muted: #94a3b8; --border: #334155; --error: #f87171; } }
body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: var(--bg); color: var(--text); font: 15px/1.5 system-ui, sans-serif; }
main { box-sizing: border-box; width: 100%; max-width: 380px; margin: 16px; padding: 24px; background: var(--card); border: 1px solid var(--border); border-radius: 12px; }
h1 { margin: 0 0 12px; font-size: 18px; }
p { margin: 0 0 12px; }
.muted { color: var(--muted); font-size: 13px; }
.error { color: var(--error); }
label { display: block; margin: 12px 0 4px; font-size: 13px; font-weight: 600; }
input[type=text], input[type=password] { box-sizing: border-box; width: 100%; padding: 8px 10px; border: 1px solid var(--border); border-radius: 8px; background: transparent; color: inherit; font: inherit; }
.check { display: flex; gap: 8px; align-items: flex-start; margin-top: 16px; font-weight: normal; }
.check span { display: block; }
.buttons { display: flex; gap: 8px; margin-top: 20px; }
button { flex: 1; padding: 8px; border-radius: 8px; border: 1px solid var(--border); background: transparent; color: inherit; font: inherit; cursor: pointer; }
button[value=allow] { background: var(--accent); border-color: var(--accent); color: #fff; }
</style>
</head>
<body>
<main>
<h1>{{.Text.Title}}</h1>
{{if .ClientID}}
<p><strong>{{.Client.Name}}</strong> {{.Text.Wants}}</p>
<p class="muted">{{.Text.Back}} <strong>{{.Host}}</strong>{{if .Loopback}}, {{.Text.Local}}{{end}}</p>
{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}
<form method="post" action="/authorize">
<input type="hidden" name="response_type" value="code">
<input type="hidden" name="client_id" value="{{.ClientID}}">
<input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
<input type="hidden" name="state" value="{{.State}}">
<input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
<input type="hidden" name="code_challenge_method" value="S256">
<label for="name">{{.Text.Name}}</label>
<input id="name" name="name" type="text" autocomplete="username" autocapitalize="none" value="{{.Name}}" required>
<label for="password">{{.Text.Password}}</label>
<input id="password" name="password" type="password" autocomplete="current-password" required>
<label class="check"><input type="checkbox" name="read_only"{{if .ReadOnly}} checked{{end}}><span>{{.Text.ReadOnly}}<span class="muted">{{.Text.ReadOnlyHelp}}</span></span></label>
<div class="buttons">
<button type="submit" name="decision" value="allow">{{.Text.Allow}}</button>
<button type="submit" name="decision" value="deny" formnovalidate>{{.Text.Deny}}</button>
</div>
</form>
{{else}}
<p class="error" role="alert">{{.Error}}</p>
{{end}}
</main>
</body>
</html>
`))
