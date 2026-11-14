// Package apiclient is the CLI's client of the API: client.gen.go is generated from docs/swagger.yaml by make openapi,
// this file keeps the server and the session of notes login.
package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var ErrNoSession = errors.New("not logged in: run notes login <url>")

// Session is the server and the session token of notes login, or of NOTES_URL and NOTES_SESSION.
type Session struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// SessionFile keeps the session, readable by the user only: the token opens the whole account.
func SessionFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "notes", "session.json"), nil
}

// Read takes NOTES_URL and NOTES_SESSION first, each on its own, for an agent that cannot run notes login where it
// runs, then the file; what is missing stays empty.
func Read() Session {
	var session Session
	if file, err := SessionFile(); err == nil {
		if data, err := os.ReadFile(file); err == nil {
			_ = json.Unmarshal(data, &session)
		}
	}
	if url := strings.TrimSpace(os.Getenv("NOTES_URL")); url != "" {
		session.URL = url
	}
	if token := strings.TrimSpace(os.Getenv("NOTES_SESSION")); token != "" {
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
	return os.WriteFile(file, data, 0o600)
}

// Client talks to the session's server as its account.
func (s Session) Client() (*ClientWithResponses, error) {
	if s.URL == "" || s.Token == "" {
		return nil, ErrNoSession
	}
	return NewClientWithResponses(s.URL, WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
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
// commands skip.
func Deleted(response *http.Response, body []byte) (bool, error) {
	switch response.StatusCode {
	case http.StatusNoContent:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, Fail(response, body)
}
