// Package apiclient is the CLI's client of the API: client.gen.go is generated from docs/swagger.yaml by make openapi,
// this file keeps the server and the session of tide login.
package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyril-fl/tide/config"
)

var (
	ErrNoSession = errors.New("not logged in: run " + config.Current.Name + " login <url>")
	ErrTimeout   = errors.New("the server did not answer in time")
)

// Timeout bounds a call to the API, so a server that goes silent does not freeze a command or the MCP; Transfer is
// the longer limit Long gives an import, an export or a file.
var Timeout, Transfer = 60 * time.Second, 10 * time.Minute

// New is the API client of every command: a call ends after Timeout, or at the deadline its ctx already has.
func New(server string, opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(server, append(opts, WithHTTPClient(doer{}))...)
}

// Long gives ctx the limit of a big transfer instead of Timeout.
func Long(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, Transfer)
}

type doer struct{}

func (doer) Do(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: Timeout}
	if _, ok := req.Context().Deadline(); ok {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return nil, fmt.Errorf("%w (%s %s): %w", ErrTimeout, req.Method, req.URL.Path, err)
	}
	return response, err
}

// Session is the server and the session token of tide login, or of TIDE_URL and TIDE_SESSION.
type Session struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// SessionFile keeps the session, readable by the user only: the token opens the whole account. Read and logout take
// the one of config.File, which may be the old ~/.config/notes.
func SessionFile() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// Read takes TIDE_URL and TIDE_SESSION first, each on its own, for an agent that cannot run tide login where it
// runs, then the file; what is missing stays empty.
func Read() Session {
	var session Session
	if file, err := config.File("session.json"); err == nil {
		if data, err := os.ReadFile(file); err == nil {
			_ = json.Unmarshal(data, &session)
		}
	}
	if url := strings.TrimSpace(os.Getenv(config.Env("URL"))); url != "" {
		session.URL = url
	}
	if token := strings.TrimSpace(os.Getenv(config.Env("SESSION"))); token != "" {
		session.Token = token
	}
	return session
}

// Save writes the session to SessionFile, readable by the user only.
func (s Session) Save() error {
	file, err := SessionFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// TODO: the old session of ~/.config/notes would come back after a logout, remove this with config.File's
	// fallback once the server is migrated to Tide.
	old, _ := config.File("session.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return err
	}
	if old != file {
		_ = os.Remove(old)
	}
	return nil
}

// Client talks to the session's server as its account.
func (s Session) Client() (*ClientWithResponses, error) {
	if s.URL == "" || s.Token == "" {
		return nil, ErrNoSession
	}
	return New(s.URL, WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+s.Token)
		return nil
	}))
}

// Fail is the error of an answer the caller did not expect: the API's message, else the HTTP status.
func Fail(response *http.Response, body []byte) error {
	var failure RespondFailure
	if json.Unmarshal(body, &failure) == nil && failure.Error != "" {
		return errors.New(failure.Error)
	}
	return errors.New(response.Status)
}

// Deleted reads the answer of a DELETE: true when deleted, false when there was nothing to delete, which the batch
// commands skip. A server older than the CLI answers 204 instead of what it deleted.
func Deleted(response *http.Response, body []byte) (bool, error) {
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, Fail(response, body)
}
