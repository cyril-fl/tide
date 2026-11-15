package millflagseditnote

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/hashtag"
	"github.com/spf13/pflag"
)

type mill struct {
	plumprimitive.Mill
	cache

	ctx    context.Context
	client *apiclient.ClientWithResponses
	flags  *pflag.FlagSet
	id     string
}

// Input is the note to rewrite, for PATCH /api/notes/{id}.
type Input struct {
	ID      string
	Content string
	Tags    []string
}

type cache struct {
	input       Input
	content     string
	addTags     []string
	removeTags  []string
	tagFromList bool
	tags        []apiclient.DomainTag
	version     time.Time
}

type Template struct {
	Ctx    context.Context
	Client *apiclient.ClientWithResponses
	Flags  *pflag.FlagSet
	ID     string
}

func New(params Template) *mill {
	return &mill{
		ctx:    params.Ctx,
		client: params.Client,
		flags:  params.Flags,
		id:     params.ID,
	}
}

// Resolve starts from the current note, applies the flags, and opens a form when no change was given.
func (m *mill) Resolve() Input {
	m.readFlags()
	m.loadNote()
	m.applyFlags()
	m.loadTags()
	m.askMissing()
	return m.cache.input
}

// Version is the note's updated_at when it was read, so the save is refused if it changed while the form was open.
func (m *mill) Version() time.Time {
	return m.cache.version
}

// Methods

func (m *mill) readFlags() {
	var errs [4]error
	m.cache.content, errs[0] = m.flags.GetString("content")
	m.cache.addTags, errs[1] = m.flags.GetStringSlice("tag")
	m.cache.removeTags, errs[2] = m.flags.GetStringSlice("untag")
	m.cache.tagFromList, errs[3] = m.flags.GetBool("tag-from-list")
	m.SetErr(errors.Join(errs[:]...))
}

// loadNote reads the note through GET /api/notes/{id}, and its version.
func (m *mill) loadNote() {
	if m.HasErr() {
		return
	}

	response, err := m.client.GetApiNotesIdWithResponse(m.ctx, m.id)
	if err != nil {
		m.SetErr(err)
		return
	}
	if response.JSON200 == nil {
		m.SetErr(apiclient.Fail(response.HTTPResponse, response.Body))
		return
	}
	current := response.JSON200
	m.cache.input = Input{ID: current.Item.Id, Content: apiclient.Deref(current.Content), Tags: apiclient.Deref(current.Tags)}
	// A note never edited has no updated_at: its version is created_at, as patchnote compares it.
	version := current.Item.CreatedAt
	if current.Item.UpdatedAt != nil {
		version = *current.Item.UpdatedAt
	}
	m.cache.version, err = time.Parse(time.RFC3339Nano, version)
	m.SetErr(err)
}

func (m *mill) applyFlags() {
	if m.HasErr() {
		return
	}

	if m.flags.Changed("content") {
		m.cache.input.Content = m.cache.content
	}
	m.cache.input.Tags = mergeTags(hashtag.Parse(m.cache.input.Content), m.cache.addTags, m.cache.removeTags)
}

func (m *mill) loadTags() {
	if m.HasErr() || !m.cache.tagFromList {
		return
	}

	tags, err := apiclient.Tags(m.ctx, m.client)
	m.cache.tags = tags
	m.SetErr(err)
}

// hasChanges reports whether a flag already says what to change, in which case the content is not asked.
func (m *mill) hasChanges() bool {
	return m.flags.Changed("content") || len(m.cache.addTags) > 0 || len(m.cache.removeTags) > 0
}

// Helpers

// mergeTags adds then removes tags, case-insensitively like the tags index; current order and spelling are kept.
func mergeTags(current, add, remove []string) []string {
	has := func(values []string, value string) bool {
		return slices.ContainsFunc(values, func(v string) bool { return strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(value)) })
	}

	var result []string
	for _, tag := range append(slices.Clone(current), add...) {
		if strings.TrimSpace(tag) != "" && !has(result, tag) && !has(remove, tag) {
			result = append(result, tag)
		}
	}
	return result
}
