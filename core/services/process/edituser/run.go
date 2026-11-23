package edituser

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/adduser"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

// MaxRetentionDays keeps the trash retention a sane number of days, ten years.
const MaxRetentionDays = 3650

// MaxSnippets and MaxSnippetSize keep the snippets of an account a small list.
const (
	MaxSnippets    = 100
	MaxSnippetSize = 64 << 10
)

// MaxSavedSearches and MaxSavedSearchIDs keep the saved searches of an account a small list, each with a short
// enough set of tags/folders that a WHERE ... IN (?) stays cheap.
const (
	MaxSavedSearches  = 100
	MaxSavedSearchIDs = 50
)

var (
	ErrInvalidLocale      = errors.New("unknown locale")
	ErrInvalidTheme       = errors.New("unknown theme")
	ErrInvalidColorMode   = errors.New("unknown color mode")
	ErrInvalidView        = errors.New("unknown view")
	ErrInvalidSort        = errors.New("unknown sort")
	ErrInvalidPlan        = errors.New("unknown plan")
	ErrInvalidAfterMerge  = errors.New("unknown choice after a merge")
	ErrInvalidRetention   = fmt.Errorf("the trash retention is between 0 and %d days", MaxRetentionDays)
	ErrInvalidNeighbors   = errors.New("unknown limit of neighbors")
	ErrInvalidAvatar      = errors.New("the avatar is not an image of the account")
	ErrInvalidSnippet     = fmt.Errorf("a snippet needs a name, at most %d of them of %d bytes each", MaxSnippets, MaxSnippetSize)
	ErrInvalidSavedSearch = fmt.Errorf("a saved search needs a name and a known tag/folder mode, at most %d of them with %d ids per list, and any_tag_ids only beside tag_ids in all mode", MaxSavedSearches, MaxSavedSearchIDs)
	ErrInvalidSidebar     = errors.New("the sidebar sections are known lists, each at most once")
	ErrNoSavedSearches    = errors.New("the plan of this account does not include savedSearches")
	ErrStale              = errors.New("account settings changed elsewhere since they were read")
)

// Input holds the settings to change: a nil one is kept.
type Input struct {
	Locale             *string                  `json:"locale,omitempty" binding:"optional"`
	Theme              *string                  `json:"theme,omitempty" binding:"optional"`
	ColorMode          *string                  `json:"color_mode,omitempty" binding:"optional"`
	View               *string                  `json:"view,omitempty" binding:"optional"`
	Sort               *string                  `json:"sort,omitempty" binding:"optional"` // see domain.SortKeys, "" for the neutral order
	Plan               *string                  `json:"plan,omitempty" binding:"optional"`
	TrashRetentionDays *int                     `json:"trash_retention_days,omitempty" binding:"optional"`
	NeighborsLimit     *int                     `json:"neighbors_limit,omitempty" binding:"optional"` // one of domain.NeighborsLimits
	AfterMerge         *string                  `json:"after_merge,omitempty" binding:"optional"`     // one of domain.AfterMerges
	NotePreviews       *bool                    `json:"note_previews,omitempty" binding:"optional"`
	Name               *string                  `json:"name,omitempty" binding:"optional"`
	Avatar             *string                  `json:"avatar,omitempty" binding:"optional"`           // the id of an image of the account, "" for none
	Snippets           *[]domain.Snippet        `json:"snippets,omitempty" binding:"optional"`         // the whole list, it replaces the saved one
	SavedSearches      *[]domain.SavedSearch    `json:"saved_searches,omitempty" binding:"optional"`   // the whole list, it replaces the saved one
	SidebarSections    *[]domain.SidebarSection `json:"sidebar_sections,omitempty" binding:"optional"` // the whole list, in order
	// UpdatedAt is the updated_at of the account as read: a 409 if its settings changed since, nothing written.
	UpdatedAt *time.Time `json:"updated_at,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	result domain.User
}

// Process changes the settings of the account of ctx.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.checkInput()
	p.checkPlan()
	p.checkAvatar()
	p.updateUser()
}

// Getters & Setters

// GetResult returns the account with its new settings.
func (p *run) GetResult() domain.User {
	return p.cache.result
}

// Methods
func (p *run) checkInput() {
	switch {
	case !valid(p.input.Locale, domain.Locales):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidLocale, *p.input.Locale))
	case !valid(p.input.Theme, domain.Themes):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidTheme, *p.input.Theme))
	case !valid(p.input.ColorMode, domain.ColorModes):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidColorMode, *p.input.ColorMode))
	case !valid(p.input.View, domain.Views):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidView, *p.input.View))
	case p.input.Sort != nil && !validSort(*p.input.Sort):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidSort, *p.input.Sort))
	case !valid(p.input.Plan, domain.Plans):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidPlan, *p.input.Plan))
	case !valid(p.input.AfterMerge, domain.AfterMerges):
		p.SetErr(fmt.Errorf("%w: %q", ErrInvalidAfterMerge, *p.input.AfterMerge))
	case p.input.TrashRetentionDays != nil && (*p.input.TrashRetentionDays < 0 || *p.input.TrashRetentionDays > MaxRetentionDays):
		p.SetErr(fmt.Errorf("%w: %d", ErrInvalidRetention, *p.input.TrashRetentionDays))
	case p.input.NeighborsLimit != nil && !slices.Contains(domain.NeighborsLimits, *p.input.NeighborsLimit):
		p.SetErr(fmt.Errorf("%w: %d", ErrInvalidNeighbors, *p.input.NeighborsLimit))
	case p.input.Name != nil && strings.TrimSpace(*p.input.Name) == "":
		p.SetErr(adduser.ErrNoName)
	case !validSnippets(p.input.Snippets):
		p.SetErr(ErrInvalidSnippet)
	case !validSavedSearches(p.input.SavedSearches):
		p.SetErr(ErrInvalidSavedSearch)
	case !validSidebar(p.input.SidebarSections):
		p.SetErr(ErrInvalidSidebar)
	}
}

// checkPlan refuses saved searches to an account whose plan, as this edit leaves it, lacks them, as plan.Require does
// for their routes.
func (p *run) checkPlan() {
	if p.HasErr() || p.input.SavedSearches == nil {
		return
	}

	var plan string
	if p.input.Plan != nil {
		plan = *p.input.Plan
	} else {
		err := p.db.NewSelect().Model((*domain.User)(nil)).Column("plan").Where("id = ?", domain.UserID(p.ctx)).Scan(p.ctx, &plan)
		if err != nil {
			p.SetErr(err)
			return
		}
	}
	if !domain.Unlocked("savedSearches", plan) {
		p.SetErr(ErrNoSavedSearches)
	}
}

func (p *run) checkAvatar() {
	if p.HasErr() || p.input.Avatar == nil || *p.input.Avatar == "" {
		return
	}

	// id::text: a value that is not a uuid is not found, rather than a failed query.
	found, err := p.db.NewSelect().Model((*domain.File)(nil)).
		Where("id::text = ? AND user_id = ? AND content_type LIKE 'image/%'", *p.input.Avatar, domain.UserID(p.ctx)).
		Exists(p.ctx)
	if err == nil && !found {
		err = fmt.Errorf("%w: %q", ErrInvalidAvatar, *p.input.Avatar)
	}
	p.SetErr(err)
}

func (p *run) updateUser() {
	if p.HasErr() {
		return
	}

	// In a transaction, a savepoint: a name already taken must not abort the caller's.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// clock_timestamp, not now(): two edits in one transaction still get two versions.
		query := tx.NewUpdate().
			Model(&p.cache.result).
			Set("updated_at = clock_timestamp()").
			Where("id = ?", domain.UserID(p.ctx)).
			Returning("?Columns")
		if p.input.UpdatedAt != nil {
			query.Where("updated_at = ?", *p.input.UpdatedAt)
		}
		if p.input.Locale != nil {
			query.Set("locale = ?", *p.input.Locale)
		}
		if p.input.Theme != nil {
			query.Set("theme = ?", *p.input.Theme)
		}
		if p.input.ColorMode != nil {
			query.Set("color_mode = ?", *p.input.ColorMode)
		}
		if p.input.View != nil {
			query.Set("view = ?", *p.input.View)
		}
		if p.input.Sort != nil {
			query.Set("sort = ?", *p.input.Sort)
		}
		if p.input.Plan != nil {
			query.Set("plan = ?", *p.input.Plan)
		}
		if p.input.TrashRetentionDays != nil {
			query.Set("trash_retention_days = ?", *p.input.TrashRetentionDays)
		}
		if p.input.NeighborsLimit != nil {
			query.Set("neighbors_limit = ?", *p.input.NeighborsLimit)
		}
		if p.input.AfterMerge != nil {
			query.Set("after_merge = ?", *p.input.AfterMerge)
		}
		if p.input.NotePreviews != nil {
			query.Set("note_previews = ?", *p.input.NotePreviews)
		}
		if p.input.Name != nil {
			query.Set("name = ?", strings.TrimSpace(*p.input.Name))
		}
		if p.input.Avatar != nil {
			query.Set("avatar = NULLIF(?, '')::uuid", *p.input.Avatar)
		}

		if p.input.Snippets != nil {
			// Read under the update's lock: the dates of each snippet come from the saved list.
			var saved domain.Snippets
			err := tx.NewSelect().Model((*domain.User)(nil)).Column("snippets").
				Where("id = ?", domain.UserID(p.ctx)).For("UPDATE").Scan(ctx, &saved)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			sealed, err := stampSnippets(saved, *p.input.Snippets, time.Now()).Seal(domain.UserID(p.ctx))
			if err != nil {
				return err
			}
			query.Set("snippets = ?", sealed)
		}

		if p.input.SavedSearches != nil {
			sealed, err := domain.SavedSearches(*p.input.SavedSearches).Seal(domain.UserID(p.ctx))
			if err != nil {
				return err
			}
			query.Set("saved_searches = ?", sealed)
		}

		if p.input.SidebarSections != nil {
			data, err := json.Marshal(*p.input.SidebarSections)
			if err != nil {
				return err
			}
			query.Set("sidebar_sections = ?", string(data))
		}

		var pgErr pgdriver.Error
		err := query.Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) && p.input.UpdatedAt != nil {
			return ErrStale
		}
		if errors.As(err, &pgErr) && pgErr.Field('C') == "23505" {
			return fmt.Errorf("%w: %q", adduser.ErrNameTaken, strings.TrimSpace(*p.input.Name))
		}
		return err
	}))
}

// Helpers

// valid accepts a setting left out, or one of choices.
func valid(value *string, choices []string) bool {
	return value == nil || slices.Contains(choices, *value)
}

func validSort(sort string) bool {
	_, ok := domain.SortList(sort)
	return ok
}

// stampSnippets dates the snippets to save from the saved ones of the same name: a new name is created now (or keeps
// the created_at it brings, a rename or an import), a changed content updated now, an unchanged one keeps its dates.
func stampSnippets(saved, next []domain.Snippet, now time.Time) domain.Snippets {
	now = now.UTC().Truncate(time.Second)
	stamped := make(domain.Snippets, len(next))
	for i, snippet := range next {
		at := slices.IndexFunc(saved, func(s domain.Snippet) bool { return s.Name == snippet.Name })
		switch {
		case at < 0:
			if snippet.CreatedAt.IsZero() {
				snippet.CreatedAt = now
			}
			snippet.UpdatedAt = now
		case saved[at].Content != snippet.Content:
			snippet.CreatedAt, snippet.UpdatedAt = saved[at].CreatedAt, now
		default:
			snippet.CreatedAt, snippet.UpdatedAt = saved[at].CreatedAt, saved[at].UpdatedAt
		}
		stamped[i] = snippet
	}
	return stamped
}

// validSnippets accepts snippets left out, or a short list of named ones.
func validSnippets(snippets *[]domain.Snippet) bool {
	if snippets == nil {
		return true
	}
	if len(*snippets) > MaxSnippets {
		return false
	}
	for _, snippet := range *snippets {
		if strings.TrimSpace(snippet.Name) == "" || len(snippet.Content) > MaxSnippetSize {
			return false
		}
	}
	return true
}

// validSavedSearches accepts saved searches left out, or a short list of named ones with a known tag/folder mode,
// short enough lists of tag/folder ids, and AnyTagIDs only beside TagIDs in "all" mode.
func validSavedSearches(searches *[]domain.SavedSearch) bool {
	if searches == nil {
		return true
	}
	if len(*searches) > MaxSavedSearches {
		return false
	}
	byID := map[string]domain.SavedSearch{}
	for _, search := range *searches {
		byID[search.ID] = search
	}
	for _, search := range *searches {
		switch {
		case !validParent(search, byID):
			return false
		case strings.TrimSpace(search.ID) == "" || strings.TrimSpace(search.Name) == "":
			return false
		case search.TagMode != "" && !slices.Contains(domain.TagModes, search.TagMode):
			return false
		case search.FolderMode != "" && !slices.Contains(domain.FolderModes, search.FolderMode):
			return false
		case !ValidSavedSearchIDs(search):
			return false
		case search.TagMode != "all" && len(search.TagIDs) > 0 && len(search.AnyTagIDs) > 0: // two "one of" lists: TagIDs in "all" mode
			return false
		}
	}
	return true
}

// validParent is true when search has no parent, or one that is a group of the list and does not lead back to search.
func validParent(search domain.SavedSearch, byID map[string]domain.SavedSearch) bool {
	for id, steps := search.ParentID, 0; id != ""; steps++ {
		parent, ok := byID[id]
		if !ok || !parent.Group || id == search.ID || steps > len(byID) {
			return false
		}
		id = parent.ParentID
	}
	return true
}

// ValidSavedSearchIDs is true when each list of tag or folder ids of search is short enough.
func ValidSavedSearchIDs(search domain.SavedSearch) bool {
	for _, ids := range [][]string{search.TagIDs, search.AnyTagIDs, search.NoTagIDs, search.FolderIDs} {
		if len(ids) > MaxSavedSearchIDs {
			return false
		}
	}
	return true
}

// validSidebar accepts sidebar sections left out, or known ones each at most once.
func validSidebar(sections *[]domain.SidebarSection) bool {
	if sections == nil {
		return true
	}
	seen := map[string]bool{}
	for _, section := range *sections {
		if !slices.Contains(domain.SidebarSectionIDs, section.ID) || seen[section.ID] {
			return false
		}
		seen[section.ID] = true
	}
	return true
}
