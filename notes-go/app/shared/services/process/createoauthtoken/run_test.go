package createoauthtoken_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/password"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createoauthcode"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createoauthtoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createschema"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createsession"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/deleteapitoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/listapitokens"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readapitoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/refreshoauthtoken"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/testaccount"
	"github.com/uptrace/bun"
)

// A connection from the consent to its revocation: a code that serves once and only with its verifier, an access token
// that reads as the account, a refresh that kills the old tokens, a revocation that kills both.
func TestOAuthConnection(t *testing.T) {
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

	var name string
	if err := tx.NewUpdate().Model((*domain.User)(nil)).Set("password_hash = ?", password.Hash("correct horse")).
		Where("id = ?", domain.UserID(ctx)).Returning("name").Scan(ctx, &name); err != nil {
		t.Fatal(err)
	}

	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	asked := createoauthcode.Input{Name: name, ClientID: "client-a", RedirectURI: "https://chatgpt.com/cb", CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]), ReadOnly: true}

	wrong := asked
	wrong.Password = "wrong"
	consent := createoauthcode.Process(ctx, tx, wrong)
	consent.Run()
	if !errors.Is(consent.GetErr(), createsession.ErrBadCredentials) {
		t.Errorf("wrong password: err = %v, want %v", consent.GetErr(), createsession.ErrBadCredentials)
	}

	code := func() string {
		t.Helper()
		right := asked
		right.Password = "correct horse"
		consent := createoauthcode.Process(ctx, tx, right)
		consent.Run()
		if consent.HasErr() {
			t.Fatalf("consent: %v", consent.GetErr())
		}
		return consent.GetResult()
	}
	exchange := createoauthtoken.Input{ClientID: "client-a", ClientName: "ChatGPT", RedirectURI: "https://chatgpt.com/cb", CodeVerifier: verifier}

	// Another client, another redirect, another verifier: refused, and the code is spent.
	for _, change := range []func(*createoauthtoken.Input){
		func(in *createoauthtoken.Input) { in.ClientID = "client-b" },
		func(in *createoauthtoken.Input) { in.RedirectURI = "https://evil.example/cb" },
		func(in *createoauthtoken.Input) { in.CodeVerifier = strings.Repeat("w", 43) },
	} {
		input := exchange
		input.Code = code()
		change(&input)
		process := createoauthtoken.Process(ctx, tx, input)
		process.Run()
		if !errors.Is(process.GetErr(), createoauthtoken.ErrInvalidGrant) {
			t.Errorf("exchange %+v: err = %v, want %v", input, process.GetErr(), createoauthtoken.ErrInvalidGrant)
		}
	}

	exchange.Code = code()
	process := createoauthtoken.Process(ctx, tx, exchange)
	process.Run()
	if process.HasErr() {
		t.Fatalf("exchange: %v", process.GetErr())
	}
	tokens := process.GetResult()
	if !strings.HasPrefix(tokens.AccessToken, "notes_") || tokens.RefreshToken == "" || tokens.ExpiresIn != 3600 || tokens.TokenType != "Bearer" {
		t.Errorf("tokens = %+v, want a notes_ access token of an hour and a refresh token", tokens)
	}
	again := createoauthtoken.Process(ctx, tx, exchange)
	again.Run()
	if !errors.Is(again.GetErr(), createoauthtoken.ErrInvalidGrant) {
		t.Errorf("code used twice: err = %v, want %v", again.GetErr(), createoauthtoken.ErrInvalidGrant)
	}

	read := readapitoken.Process(ctx, tx, tokens.AccessToken)
	read.Run()
	if read.HasErr() || read.GetResult().ID != domain.UserID(ctx) || !read.GetReadOnly() {
		t.Fatalf("access token: %+v read-only %v, %v; want the account, read-only", read.GetResult(), read.GetReadOnly(), read.GetErr())
	}

	list := listapitokens.Process(ctx, tx)
	list.Run()
	if list.HasErr() || len(list.GetResult()) != 1 || !list.GetResult()[0].OAuth || list.GetResult()[0].Name != "ChatGPT" || list.GetResult()[0].ExpiresAt.IsZero() {
		t.Fatalf("list = %+v, %v; want one OAuth connection named ChatGPT, with its expiry", list.GetResult(), list.GetErr())
	}

	// The access token dies after its hour, the connection stays.
	if _, err := tx.NewUpdate().Model((*domain.APIToken)(nil)).Set("access_expires_at = current_timestamp - interval '1 minute'").
		Where("user_id = ?", domain.UserID(ctx)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if refused(ctx, tx, tokens.AccessToken) == nil {
		t.Errorf("access token after its hour: read, want refused")
	}

	refresh := refreshoauthtoken.Process(ctx, tx, refreshoauthtoken.Input{RefreshToken: tokens.RefreshToken, ClientID: "client-b"})
	refresh.Run()
	if !errors.Is(refresh.GetErr(), createoauthtoken.ErrInvalidGrant) {
		t.Errorf("refresh by another client: err = %v, want %v", refresh.GetErr(), createoauthtoken.ErrInvalidGrant)
	}
	refresh = refreshoauthtoken.Process(ctx, tx, refreshoauthtoken.Input{RefreshToken: tokens.RefreshToken, ClientID: "client-a"})
	refresh.Run()
	if refresh.HasErr() {
		t.Fatalf("refresh: %v", refresh.GetErr())
	}
	fresh := refresh.GetResult()
	if fresh.AccessToken == tokens.AccessToken || fresh.RefreshToken == tokens.RefreshToken {
		t.Errorf("refresh gave the same tokens back")
	}
	if err := refused(ctx, tx, fresh.AccessToken); err != nil {
		t.Errorf("new access token: %v", err)
	}
	old := refreshoauthtoken.Process(ctx, tx, refreshoauthtoken.Input{RefreshToken: tokens.RefreshToken, ClientID: "client-a"})
	old.Run()
	if !errors.Is(old.GetErr(), createoauthtoken.ErrInvalidGrant) {
		t.Errorf("old refresh token: err = %v, want %v", old.GetErr(), createoauthtoken.ErrInvalidGrant)
	}

	revoke := deleteapitoken.Process(ctx, tx, list.GetResult()[0].ID)
	revoke.Run()
	if revoke.HasErr() {
		t.Fatalf("revoke: %v", revoke.GetErr())
	}
	if refused(ctx, tx, fresh.AccessToken) == nil {
		t.Errorf("access token after the revocation: read, want refused")
	}
	gone := refreshoauthtoken.Process(ctx, tx, refreshoauthtoken.Input{RefreshToken: fresh.RefreshToken, ClientID: "client-a"})
	gone.Run()
	if !errors.Is(gone.GetErr(), createoauthtoken.ErrInvalidGrant) {
		t.Errorf("refresh after the revocation: err = %v, want %v", gone.GetErr(), createoauthtoken.ErrInvalidGrant)
	}
}

// refused is the error of reading an access token, nil when it works.
func refused(ctx context.Context, tx bun.Tx, token string) error {
	read := readapitoken.Process(ctx, tx, token)
	read.Run()
	return read.GetErr()
}
