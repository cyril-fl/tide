package deleteapitoken

import (
	"context"
	"errors"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("no API token with this id")

type run struct {
	plumprimitive.Process

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	id  string
}

// Process revokes an API token of the account of ctx: it stops working at once.
func Process(ctx context.Context, db bun.IDB, id string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		id:  id,
	}
}

func (p *run) Run() {
	result, err := p.db.NewDelete().Model((*domain.APIToken)(nil)).Where("id = ? AND user_id = ?", p.id, domain.UserID(p.ctx)).Exec(p.ctx)
	if err == nil {
		var gone int64
		if gone, err = result.RowsAffected(); err == nil && gone == 0 {
			err = ErrNotFound
		}
	}
	p.SetErr(err)
}
