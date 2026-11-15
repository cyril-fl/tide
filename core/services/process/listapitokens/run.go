package listapitokens

import (
	"context"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
}

type cache struct {
	tokens []domain.APIToken
}

// Process lists the API tokens of the account of ctx, the newest first. Their hash never leaves the base (json:"-").
func Process(ctx context.Context, db bun.IDB) *run {
	return &run{
		ctx: ctx,
		db:  db,
	}
}

func (p *run) Run() {
	err := p.db.NewSelect().Model(&p.cache.tokens).Where("user_id = ?", domain.UserID(p.ctx)).Order("created_at DESC").Scan(p.ctx)
	p.SetErr(err)
}

// Getters & Setters

func (p *run) GetResult() []domain.APIToken {
	return p.cache.tokens
}
