package purgesessions

import (
	"context"
	"testing"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/testaccount"
)

func TestPurgeExpiredOnly(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	now := time.Now()
	sessions := []domain.Session{
		{UserID: domain.UserID(ctx), TokenHash: "expired-" + now.String(), ExpiresAt: now.Add(-time.Minute)},
		{UserID: domain.UserID(ctx), TokenHash: "live-" + now.String(), ExpiresAt: now.Add(time.Hour)},
	}
	if _, err := tx.NewInsert().Model(&sessions).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	name, err := secret.Seal(domain.UserID(ctx), "token")
	if err != nil {
		t.Fatal(err)
	}
	tokens := []domain.APIToken{
		{UserID: domain.UserID(ctx), Name: name, TokenHash: "expired-token-" + now.String(), ExpiresAt: now.Add(-time.Minute)},
		{UserID: domain.UserID(ctx), Name: name, TokenHash: "live-token-" + now.String(), ExpiresAt: now.Add(time.Hour)},
		{UserID: domain.UserID(ctx), Name: name, TokenHash: "never-token-" + now.String()},
	}
	if _, err := tx.NewInsert().Model(&tokens).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	purge := Process(ctx, tx)
	purge.Run()
	if purge.HasErr() {
		t.Fatalf("purge: %v", purge.GetErr())
	}
	// Other expired sessions of the base may be purged too: at least ours.
	if purge.GetResult() < 2 {
		t.Errorf("purged = %d, want at least 2", purge.GetResult())
	}

	var left []string
	if err := tx.NewSelect().Model((*domain.Session)(nil)).Column("token_hash").Where("user_id = ?", domain.UserID(ctx)).Scan(ctx, &left); err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0] != sessions[1].TokenHash {
		t.Errorf("sessions left = %v, want only %q", left, sessions[1].TokenHash)
	}

	var hashes []string
	if err := tx.NewSelect().Model((*domain.APIToken)(nil)).Column("token_hash").Where("user_id = ?", domain.UserID(ctx)).Order("token_hash").Scan(ctx, &hashes); err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[0] != tokens[1].TokenHash || hashes[1] != tokens[2].TokenHash {
		t.Errorf("API tokens left = %v, want live and never", hashes)
	}
}
