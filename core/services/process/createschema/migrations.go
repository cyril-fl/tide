package createschema

import "github.com/uptrace/bun"

// TODO: a stopgap until v2 has real migrations: one-time queries for what createschema cannot do (a renamed or
// retyped column, a changed default, data to rewrite), run once per base and recorded in the migrations table.
// Append only, never edit nor reorder one that has shipped: its id is how a base knows it ran.
var migrations = []migration{
	migrationV1_17_3,
}

type migration struct {
	id    string
	query string
}

// v1.17.3: the sort of a folder is neutral by default, "title" before.
var migrationV1_17_3 = migration{"v1.17.3-sort-neutral", `
	ALTER TABLE users ALTER COLUMN sort SET DEFAULT '';
	UPDATE users SET sort = '' WHERE sort = 'title'`}

// runMigrations runs, in the schema's transaction, the migrations this base has not run yet. The INSERT claims one
// first: a server and a CLI starting together wait on its row, and the second skips it.
func (p *run) runMigrations() {
	p.cache.steps = append(p.cache.steps, func(tx bun.Tx) {
		if p.HasErr() {
			return
		}
		_, err := tx.ExecContext(p.ctx, `CREATE TABLE IF NOT EXISTS migrations (id text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`)
		p.SetErr(err)
		for _, m := range migrations {
			if p.HasErr() {
				return
			}
			result, err := tx.ExecContext(p.ctx, `INSERT INTO migrations (id) VALUES (?) ON CONFLICT DO NOTHING`, m.id)
			if err != nil {
				p.SetErr(err)
				return
			}
			if claimed, _ := result.RowsAffected(); claimed == 1 {
				_, err = tx.ExecContext(p.ctx, m.query)
				p.SetErr(err)
			}
		}
	})
}
