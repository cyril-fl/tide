package readapitoken

import (
	"context"
	"database/sql"
	"errors"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/password"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readsession"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	token string
}

type cache struct {
	user domain.User
}

// Process finds the account of an API token that has not expired, and marks the token used now.
// It loads the key of the account: its notes open from then on. An unknown, revoked or expired token is
// readsession.ErrNoSession, the same 401 as a session.
func Process(ctx context.Context, db bun.IDB, token string) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		token: token,
	}
}

func (p *run) Run() {
	err := p.db.NewRaw(`
		WITH token AS (
			UPDATE api_tokens SET last_used_at = current_timestamp
			WHERE token_hash = ? AND (expires_at IS NULL OR expires_at > current_timestamp) RETURNING user_id
		)
		SELECT u.id, u.name, u.created_at FROM users AS u JOIN token ON token.user_id = u.id`,
		password.TokenHash(p.token)).
		Scan(p.ctx, &p.cache.user)
	if errors.Is(err, sql.ErrNoRows) {
		err = readsession.ErrNoSession
	}
	if err == nil {
		err = secret.Load(p.ctx, p.db, p.cache.user.ID)
	}
	p.SetErr(err)
}

// Getters & Setters

func (p *run) GetResult() domain.User {
	return p.cache.user
}
