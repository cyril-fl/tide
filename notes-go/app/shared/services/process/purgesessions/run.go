package purgesessions

import (
	"context"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
}

type cache struct {
	purged int64
}

// Process deletes the expired sessions and API tokens of every account: a device that never logs out (cookie cleared,
// device lost) would keep its row, and its token hash, forever. The OAuth codes never exchanged and the login
// throttles a day old go too.
func Process(ctx context.Context, db bun.IDB) *run {
	return &run{
		ctx: ctx,
		db:  db,
	}
}

func (p *run) Run() {
	p.purge((*domain.Session)(nil))
	p.purge((*domain.APIToken)(nil))
	p.purge((*domain.OAuthCode)(nil))
	p.purge((*domain.Throttle)(nil))
}

// Getters & Setters

// GetResult returns how many rows were deleted.
func (p *run) GetResult() int64 {
	return p.cache.purged
}

// Methods

// purge deletes the expired rows of model; an API token without expires_at never expires.
func (p *run) purge(model any) {
	if p.HasErr() {
		return
	}

	result, err := p.db.NewDelete().Model(model).Where("expires_at < current_timestamp").Exec(p.ctx)
	if err == nil {
		var purged int64
		purged, err = result.RowsAffected()
		p.cache.purged += purged
	}
	p.SetErr(err)
}
