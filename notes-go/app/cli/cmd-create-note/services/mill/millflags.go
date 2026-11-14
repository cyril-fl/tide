package millflagscreatenote

import (
	"context"
	"errors"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/spf13/pflag"
)

type mill struct {
	plumprimitive.Mill
	cache

	ctx    context.Context
	client *apiclient.ClientWithResponses
	flags  *pflag.FlagSet
}

// Input is the note to create, for POST /api/notes; ParentID "" is the root.
type Input struct {
	Content  string
	ParentID string
	Tags     []string
}

type cache struct {
	input          Input
	tagFromList    bool
	parentFromList bool
	tags           []apiclient.DomainTag
}

type Template struct {
	Ctx    context.Context
	Client *apiclient.ClientWithResponses
	Flags  *pflag.FlagSet
}

func New(params Template) *mill {
	return &mill{
		ctx:    params.Ctx,
		client: params.Client,
		flags:  params.Flags,
	}
}

// Resolve turns the flags into a complete note, asking in a form for what is missing.
func (m *mill) Resolve() Input {
	m.readFlags()
	m.pickParent()
	m.loadTags()
	m.askMissing()

	return m.cache.input
}

// Methods

func (m *mill) readFlags() {
	var errs [4]error

	m.cache.input.Content, errs[0] = m.flags.GetString("content")
	m.cache.input.ParentID, errs[1] = m.flags.GetString("parent")
	m.cache.input.Tags, errs[2] = m.flags.GetStringSlice("tag")
	errs[3] = m.readBoolFlags(map[string]*bool{
		"tag-from-list":    &m.cache.tagFromList,
		"parent-from-list": &m.cache.parentFromList,
	})

	m.SetErr(errors.Join(errs[:]...))
}

// pickParent asks for the parent in the folder list with --parent-from-list.
func (m *mill) pickParent() {
	if m.HasErr() || !m.cache.parentFromList {
		return
	}

	folders, err := apiclient.Folders(m.ctx, m.client)
	if err != nil {
		m.SetErr(err)
		return
	}

	id, err := huhform.PickFolder(folders)
	m.cache.input.ParentID = id
	m.SetErr(err)
}

func (m *mill) loadTags() {
	if m.HasErr() || !m.cache.tagFromList {
		return
	}

	tags, err := apiclient.Tags(m.ctx, m.client)
	m.cache.tags = tags
	m.SetErr(err)
}

// Helpers

// readBoolFlags mirrors plumcliutilsargs.GetBoolFlags, which needs a *cobra.Command and not a FlagSet.
func (m *mill) readBoolFlags(targets map[string]*bool) error {
	for name, target := range targets {
		value, err := m.flags.GetBool(name)
		if err != nil {
			return err
		}
		*target = value
	}

	return nil
}
