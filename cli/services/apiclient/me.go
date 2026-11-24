package apiclient

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// errAccountStale is the text of edituser.ErrStale the API answers: the account changed since it was read.
const errAccountStale = "account settings changed elsewhere since they were read"

// ErrNoSnippet is a snippet name the account does not have.
var ErrNoSnippet = errors.New("no snippet of that name")

// EditMe reads the account, has edit build the whole lists to save from it, and saves them through PATCH /api/me with
// the updated_at read: a write of the app or another client in between is read again and edit run over it, once.
func EditMe(ctx context.Context, client *ClientWithResponses, edit func(DomainUser) (EdituserInput, error)) (DomainUser, error) {
	for try := 0; ; try++ {
		me, err := Decode[DomainUser](client.GetApiMe(ctx))
		if err != nil {
			return me, err
		}
		input, err := edit(me)
		if err != nil {
			return me, err
		}
		input.UpdatedAt = &me.UpdatedAt
		me, err = Decode[DomainUser](client.PatchApiMe(ctx, input))
		if err == nil || err.Error() != errAccountStale || try > 0 {
			return me, err
		}
	}
}

// Snippets is the account's snippets, [] when it has none.
func Snippets(ctx context.Context, client *ClientWithResponses) ([]DomainSnippet, error) {
	me, err := Decode[DomainUser](client.GetApiMe(ctx))
	return append([]DomainSnippet{}, me.Snippets...), err
}

// SaveSnippet creates or replaces the snippet of that name, or deletes it, the others kept; it answers the snippet
// saved or deleted.
func SaveSnippet(ctx context.Context, client *ClientWithResponses, name, content string, remove bool) (DomainSnippet, error) {
	answer := DomainSnippet{Name: name, Content: content}
	_, err := EditMe(ctx, client, func(me DomainUser) (EdituserInput, error) {
		snippets := slices.Clone(me.Snippets)
		i := slices.IndexFunc(snippets, func(s DomainSnippet) bool { return s.Name == name })
		switch {
		case remove && i < 0:
			return EdituserInput{}, fmt.Errorf("%w: %q", ErrNoSnippet, name)
		case remove:
			answer = snippets[i]
			snippets = slices.Delete(snippets, i, i+1)
		case i >= 0:
			snippets[i] = answer
		default:
			snippets = append(snippets, answer)
		}
		return EdituserInput{Snippets: &snippets}, nil
	})
	return answer, err
}
