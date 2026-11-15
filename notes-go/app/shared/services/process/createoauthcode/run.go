package createoauthcode

import (
	"context"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/password"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createsession"
	"github.com/uptrace/bun"
)

// Lifetime is how long a code waits for its exchange: the client exchanges it at once.
const Lifetime = time.Minute

// Input is the consent of the OAuth page: the account's name and password, and what the client asked.
type Input struct {
	Name          string
	Password      string
	ClientID      string
	RedirectURI   string
	CodeChallenge string // S256 of the client's verifier
	ReadOnly      bool
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	user domain.User
	code string
}

// Process checks the password of the account and gives an authorization code for the client: the consent, without
// a session (createsession.ErrBadCredentials on a wrong name or password).
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.checkPassword()
	p.saveCode()
}

// Getters & Setters

// GetResult is the code, for the client only: the base keeps its hash.
func (p *run) GetResult() string {
	return p.cache.code
}

// Methods

func (p *run) checkPassword() {
	user, err := createsession.CheckPassword(p.ctx, p.db, p.input.Name, p.input.Password)
	p.cache.user = user
	p.SetErr(err)
}

func (p *run) saveCode() {
	if p.HasErr() {
		return
	}

	code := password.Token()
	row := domain.OAuthCode{
		CodeHash:      password.TokenHash(code),
		UserID:        p.cache.user.ID,
		ClientID:      p.input.ClientID,
		RedirectURI:   p.input.RedirectURI,
		CodeChallenge: p.input.CodeChallenge,
		ReadOnly:      p.input.ReadOnly,
		ExpiresAt:     time.Now().Add(Lifetime),
	}
	if _, err := p.db.NewInsert().Model(&row).Exec(p.ctx); err != nil {
		p.SetErr(err)
		return
	}
	p.cache.code = code
}
