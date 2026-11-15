package createapitoken_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/password"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createapitoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/deleteapitoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/listapitokens"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readapitoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readsession"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/secret"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/testaccount"
)

// A token from its creation to its revocation: read by its account, listed without its secret, refused once expired.
func TestTokenLifecycle(t *testing.T) {
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

	for input, want := range map[createapitoken.Input]error{
		{Name: " "}:                      createapitoken.ErrNoName,
		{Name: strings.Repeat("é", 101)}: createapitoken.ErrLongName,
		{Name: "x", ExpiresIn: "2d"}:     createapitoken.ErrInvalidLifetime,
		{Name: "x", ExpiresIn: "365d"}:   createapitoken.ErrInvalidLifetime,
	} {
		process := createapitoken.Process(ctx, tx, input)
		process.Run()
		if !errors.Is(process.GetErr(), want) {
			t.Errorf("create %+v: err = %v, want %v", input, process.GetErr(), want)
		}
	}

	create := createapitoken.Process(ctx, tx, createapitoken.Input{Name: " Claude téléphone ", ExpiresIn: "7d"})
	create.Run()
	if create.HasErr() {
		t.Fatalf("create: %v", create.GetErr())
	}
	created := create.GetResult()
	if created.APIToken.ID == "" || created.APIToken.Name != "Claude téléphone" || time.Until(created.APIToken.ExpiresAt).Round(time.Hour) != 7*24*time.Hour {
		t.Errorf("created = %+v, want an id, the name trimmed, 7 days", created.APIToken)
	}

	var stored string
	if err := tx.NewSelect().Table("api_tokens").Column("name").Where("id = ?", created.APIToken.ID).Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, secret.Prefix) || strings.Contains(stored, "Claude") {
		t.Errorf("the base keeps the name %q, want it sealed", stored)
	}

	read := readapitoken.Process(ctx, tx, created.Token)
	read.Run()
	if read.HasErr() || read.GetResult().ID != domain.UserID(ctx) {
		t.Fatalf("read = %+v, %v; want the account", read.GetResult(), read.GetErr())
	}

	if read.GetReadOnly() {
		t.Errorf("a token made without read_only reads as read-only")
	}
	readOnly := createapitoken.Process(ctx, tx, createapitoken.Input{Name: "reader", ReadOnly: true})
	readOnly.Run()
	if readOnly.HasErr() || !readOnly.GetResult().APIToken.ReadOnly {
		t.Fatalf("create read-only = %+v, %v", readOnly.GetResult().APIToken, readOnly.GetErr())
	}
	read = readapitoken.Process(ctx, tx, readOnly.GetResult().Token)
	read.Run()
	if read.HasErr() || !read.GetReadOnly() {
		t.Errorf("read a read-only token: read-only = %v, %v; want true", read.GetReadOnly(), read.GetErr())
	}
	if _, err := tx.NewDelete().Model((*domain.APIToken)(nil)).Where("id = ?", readOnly.GetResult().APIToken.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	list := listapitokens.Process(ctx, tx)
	list.Run()
	if list.HasErr() || len(list.GetResult()) != 1 || list.GetResult()[0].LastUsedAt.IsZero() || list.GetResult()[0].Name != "Claude téléphone" {
		t.Fatalf("list = %+v, %v; want the token, used, its name opened", list.GetResult(), list.GetErr())
	}
	if list.GetResult()[0].TokenHash != password.TokenHash(created.Token) {
		t.Errorf("the base keeps the hash of the token")
	}

	// Expired: refused like an unknown token.
	if _, err := tx.NewUpdate().Model((*domain.APIToken)(nil)).Set("expires_at = current_timestamp - interval '1 minute'").Where("id = ?", created.APIToken.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	read = readapitoken.Process(ctx, tx, created.Token)
	read.Run()
	if !errors.Is(read.GetErr(), readsession.ErrNoSession) {
		t.Errorf("expired: err = %v, want %v", read.GetErr(), readsession.ErrNoSession)
	}

	revoke := deleteapitoken.Process(ctx, tx, created.APIToken.ID)
	revoke.Run()
	if revoke.HasErr() {
		t.Fatalf("revoke: %v", revoke.GetErr())
	}
	revoke = deleteapitoken.Process(ctx, tx, created.APIToken.ID)
	revoke.Run()
	if !errors.Is(revoke.GetErr(), deleteapitoken.ErrNotFound) {
		t.Errorf("revoke twice: err = %v, want %v", revoke.GetErr(), deleteapitoken.ErrNotFound)
	}
}
