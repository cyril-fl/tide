package createapitoken

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/password"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// Prefix starts every API token: auth tells it from a session token by it, and a leaked one is recognizable.
const Prefix = "notes_"

// Lifetimes are the durations a token can be given; 0 is no limit.
var Lifetimes = map[string]time.Duration{
	"7d":    7 * 24 * time.Hour,
	"30d":   30 * 24 * time.Hour,
	"90d":   90 * 24 * time.Hour,
	"1y":    365 * 24 * time.Hour,
	"never": 0,
}

// DefaultLifetime is the duration of a token when none is asked.
const DefaultLifetime = "90d"

var (
	ErrNoName          = errors.New("the token needs a name")
	ErrInvalidLifetime = errors.New("expires_in must be one of 7d, 30d, 90d, 1y, never")
)

type Input struct {
	Name string `json:"name"`
	// ExpiresIn is how long the token works, 90d when left out.
	ExpiresIn string `json:"expires_in" binding:"optional" enums:"7d,30d,90d,1y,never"`
}

// Result is the new token: Token is shown once, the base keeps only its hash.
type Result struct {
	Token    string          `json:"token"`
	APIToken domain.APIToken `json:"api_token"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	lifetime time.Duration
	result   Result
}

// Process gives the account of ctx a new API token.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.ensureInput()
	p.saveToken()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

func (p *run) ensureInput() {
	if strings.TrimSpace(p.input.Name) == "" {
		p.SetErr(ErrNoName)
		return
	}
	if p.input.ExpiresIn == "" {
		p.input.ExpiresIn = DefaultLifetime
	}
	lifetime, ok := Lifetimes[p.input.ExpiresIn]
	if !ok {
		p.SetErr(ErrInvalidLifetime)
		return
	}
	p.cache.lifetime = lifetime
}

func (p *run) saveToken() {
	if p.HasErr() {
		return
	}

	p.cache.result.Token = Prefix + password.Token()
	token := &p.cache.result.APIToken
	token.UserID = domain.UserID(p.ctx)
	token.Name = strings.TrimSpace(p.input.Name)
	token.TokenHash = password.TokenHash(p.cache.result.Token)
	if p.cache.lifetime > 0 {
		token.ExpiresAt = time.Now().Add(p.cache.lifetime)
	}
	err := p.db.NewInsert().Model(token).Returning("id, created_at").Scan(p.ctx)
	p.SetErr(err)
}
