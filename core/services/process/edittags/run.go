package edittags

import (
	"context"
	"errors"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/edittag"
	"github.com/uptrace/bun"
)

var (
	ErrNoValue = errors.New("no tag value given")

	// errDryRun makes the transaction of a dry run roll back; it never leaves the process.
	errDryRun = errors.New("dry run")
)

// Renamed is a tag renamed; with a dry run, also the notes it rewrote.
type Renamed struct {
	domain.Tag
	Notes []edittag.Changed `json:"notes,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	values []string
	to     string
	merge  bool
	dry    bool
	result []Renamed
}

// Process renames each of values as to, in the order given and in one transaction: a failure renames none of them.
// Each one is refused or merged as edittag does.
func Process(ctx context.Context, db bun.IDB, values []string, to string, merge bool) *run {
	return &run{ctx: ctx, db: db, values: values, to: to, merge: merge, result: []Renamed{}}
}

// DryRun runs everything then rolls it back: the result is the one of a real call.
func (p *run) DryRun() *run {
	p.dry = true
	return p
}

func (p *run) Run() {
	if len(p.values) == 0 {
		p.SetErr(ErrNoValue)
		return
	}
	err := p.db.RunInTx(p.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, value := range p.values {
			// Inside tx the process's own transaction is a savepoint: a failure unwinds the whole batch.
			one := edittag.Process(ctx, tx, value, p.to, p.merge)
			if p.dry {
				one.DryRun()
			}
			if one.Run(); one.HasErr() {
				return one.GetErr()
			}
			p.result = append(p.result, Renamed{Tag: one.GetResult(), Notes: one.GetNotes()})
		}
		if p.dry {
			return errDryRun
		}
		return nil
	})
	if !errors.Is(err, errDryRun) {
		p.SetErr(err)
	}
}

// GetResult returns the tags renamed, in the order asked.
func (p *run) GetResult() []Renamed {
	return p.result
}
