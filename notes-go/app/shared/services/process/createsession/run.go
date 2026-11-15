package createsession

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/password"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/secret"
	"github.com/uptrace/bun"
)

// Lifetime is how long a session lasts without use: each use starts it again (readsession).
const Lifetime = 30 * 24 * time.Hour

var ErrBadCredentials = errors.New("wrong account name or password")

// dummyHash is checked when the name is unknown: the answer takes as long, and does not tell names apart.
var dummyHash = password.Hash("not a password")

type Input struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Token asks the token in the answer too, for a client without cookies (the CLI): the web keeps it HttpOnly.
	Token bool `json:"token" binding:"optional"`
}

// Result is the new session: the token goes to the client, and only there.
type Result struct {
	Token     string      `json:"token,omitempty" binding:"optional"`
	User      domain.User `json:"user"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	result Result
}

// Process logs in: the account's password checked, a session made for the device.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.checkPassword()
	p.loadKey()
	p.saveSession()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

func (p *run) checkPassword() {
	user, err := CheckPassword(p.ctx, p.db, p.input.Name, p.input.Password)
	p.cache.result.User = user
	p.SetErr(err)
}

// loadKey reads the snippets and saved searches once the key of the account opens them.
func (p *run) loadKey() {
	if p.HasErr() {
		return
	}

	user := &p.cache.result.User
	err := secret.Load(p.ctx, p.db, user.ID)
	if err == nil {
		err = p.db.NewSelect().Model(user).Column("snippets", "saved_searches").WherePK().Scan(p.ctx)
	}
	p.SetErr(err)
}

func (p *run) saveSession() {
	if p.HasErr() {
		return
	}

	p.cache.result.Token = password.Token()
	p.cache.result.ExpiresAt = time.Now().Add(Lifetime)
	session := domain.Session{UserID: p.cache.result.User.ID, TokenHash: password.TokenHash(p.cache.result.Token), ExpiresAt: p.cache.result.ExpiresAt}
	_, err := p.db.NewInsert().Model(&session).Exec(p.ctx)
	p.SetErr(err)
}

// Helpers

// CheckPassword finds the account of name and checks its password, in the same time whether the name exists or not:
// ErrBadCredentials either way. The OAuth consent logs in with it, without a session.
func CheckPassword(ctx context.Context, db bun.IDB, name, pass string) (domain.User, error) {
	var user domain.User
	err := db.NewSelect().Model(&user).ExcludeColumn("snippets", "saved_searches").Where("lower(name) = lower(?)", strings.TrimSpace(name)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		password.Verify(pass, dummyHash)
		return user, ErrBadCredentials
	}
	if err != nil {
		return user, err
	}
	if !password.Verify(pass, user.PasswordHash) {
		return user, ErrBadCredentials
	}
	return user, nil
}
