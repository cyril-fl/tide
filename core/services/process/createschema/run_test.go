package createschema

import (
	"context"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/uptrace/bun"
)

// A base made before a column of the model gets it back, with its default, when the schema is created again.
func TestAddColumns(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE users DROP COLUMN view"); err != nil {
		t.Fatal(err)
	}

	schema = Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema again: %v", schema.GetErr())
	}
	var def string
	if err := db.NewRaw("SELECT column_default FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'view' AND is_nullable = 'NO'").Scan(ctx, &def); err != nil {
		t.Fatalf("users.view not back: %v", err)
	}
	if def != "'list'::character varying" {
		t.Errorf("users.view default = %s, want 'list'", def)
	}
}

// A migration runs once per base: the second start skips it, else this CREATE TABLE would fail.
func TestMigrations(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	saved := migrations
	migrations = append(slices.Clone(migrations), migration{"test-once", "CREATE TABLE test_migration_once ()"})
	t.Cleanup(func() {
		migrations = saved
		db.ExecContext(ctx, "DROP TABLE IF EXISTS test_migration_once")
		db.ExecContext(ctx, "DELETE FROM migrations WHERE id = 'test-once'")
	})

	for range 2 {
		schema := Process(ctx, db)
		schema.Run()
		if schema.HasErr() {
			t.Fatalf("create schema: %v", schema.GetErr())
		}
	}
	var ran int
	if err := db.NewRaw("SELECT count(*) FROM migrations WHERE id IN (?)", bun.In([]string{"test-once", migrationV1_17_3.id})).Scan(ctx, &ran); err != nil || ran != 2 {
		t.Errorf("migrations recorded = %d, %v; want 2", ran, err)
	}
}
