package refreshoauthtoken

import (
	"context"

	"github.com/cyril-fl/notes/notes-go/app/shared/services/password"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createoauthtoken"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// Input is a refresh (grant_type=refresh_token).
type Input struct {
	RefreshToken string
	ClientID     string
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	result createoauthtoken.Result
}

// Process gives an OAuth connection a new access token and a new refresh token, the old ones dead at once (rotation,
// as OAuth 2.1 asks for a public client), and 90 more days. A connection revoked, expired, or of another client is
// createoauthtoken.ErrInvalidGrant.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	result, row := createoauthtoken.NewTokens()
	update, err := p.db.NewUpdate().Model(&row).
		Column("token_hash", "refresh_hash", "access_expires_at", "expires_at").
		Where("refresh_hash = ? AND client_id = ? AND oauth AND expires_at > current_timestamp",
			password.TokenHash(p.input.RefreshToken), p.input.ClientID).
		Exec(p.ctx)
	if err != nil {
		p.SetErr(err)
		return
	}
	rows, err := update.RowsAffected()
	if err == nil && rows == 0 {
		err = createoauthtoken.ErrInvalidGrant
	}
	if err != nil {
		p.SetErr(err)
		return
	}
	p.cache.result = result
}

// Getters & Setters

func (p *run) GetResult() createoauthtoken.Result {
	return p.cache.result
}
