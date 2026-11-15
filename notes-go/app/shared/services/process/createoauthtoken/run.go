package createoauthtoken

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/password"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createapitoken"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

const (
	// AccessLifetime is how long an access token works: the client refreshes it after.
	AccessLifetime = time.Hour
	// RefreshLifetime is how long a connection lasts without use: each refresh starts it again.
	RefreshLifetime = 90 * 24 * time.Hour
)

// ErrInvalidGrant is the OAuth error invalid_grant: a code or refresh token unknown, expired, used, or of another client.
var ErrInvalidGrant = errors.New("the code or refresh token is unknown, expired, already used, or of another client")

// Input is the exchange of an authorization code (grant_type=authorization_code).
type Input struct {
	Code         string
	ClientID     string
	ClientName   string // the name of the connection in the account's list of tokens
	RedirectURI  string
	CodeVerifier string
}

// Result is the token answer of OAuth: the tokens go to the client, the base keeps their hash.
type Result struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	code   domain.OAuthCode
	result Result
}

// Process exchanges an authorization code for a new connection of the account in api_tokens: an access token of an
// hour and a refresh token of 90 days. The code serves once, used or refused.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.takeCode()
	p.ensureCode()
	p.loadKey()
	p.saveToken()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

// takeCode deletes the code as it reads it: two exchanges at once cannot both get it.
func (p *run) takeCode() {
	err := p.db.NewDelete().Model(&p.cache.code).
		Where("code_hash = ? AND expires_at > current_timestamp", password.TokenHash(p.input.Code)).
		Returning("?Columns").Scan(p.ctx)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrInvalidGrant
	}
	p.SetErr(err)
}

func (p *run) ensureCode() {
	if p.HasErr() {
		return
	}

	code := p.cache.code
	if code.ClientID != p.input.ClientID || code.RedirectURI != p.input.RedirectURI || !Verifies(p.input.CodeVerifier, code.CodeChallenge) {
		p.SetErr(ErrInvalidGrant)
	}
}

// loadKey opens the key of the account, to seal the name of the connection.
func (p *run) loadKey() {
	if p.HasErr() {
		return
	}

	p.SetErr(secret.Load(p.ctx, p.db, p.cache.code.UserID))
}

func (p *run) saveToken() {
	if p.HasErr() {
		return
	}

	name, err := secret.Seal(p.cache.code.UserID, p.input.ClientName)
	if err != nil {
		p.SetErr(err)
		return
	}
	result, row := NewTokens()
	row.UserID = p.cache.code.UserID
	row.Name = name
	row.ReadOnly = p.cache.code.ReadOnly
	row.OAuth = true
	row.ClientID = p.cache.code.ClientID
	if _, err := p.db.NewInsert().Model(&row).Exec(p.ctx); err != nil {
		p.SetErr(err)
		return
	}
	p.cache.result = result
}

// Helpers

// NewTokens gives a new access token and refresh token, and the row fields they set: their hashes and expiries.
func NewTokens() (Result, domain.APIToken) {
	now := time.Now()
	result := Result{
		AccessToken:  createapitoken.Prefix + password.Token(),
		TokenType:    "Bearer",
		ExpiresIn:    int(AccessLifetime.Seconds()),
		RefreshToken: password.Token(),
	}
	row := domain.APIToken{
		TokenHash:       password.TokenHash(result.AccessToken),
		RefreshHash:     password.TokenHash(result.RefreshToken),
		AccessExpiresAt: now.Add(AccessLifetime),
		ExpiresAt:       now.Add(RefreshLifetime),
	}
	return result, row
}

// Verifies tells if verifier is the one of challenge, PKCE S256: BASE64URL(SHA256(verifier)).
func Verifies(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}
