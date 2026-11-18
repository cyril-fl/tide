package domain

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
)

// User is an account: every item and tag belongs to one.
type User struct {
	bun.BaseModel `bun:"table:users,alias:u" json:"-"`

	ID           string `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	Name         string `bun:",notnull" json:"name"` // unique case-insensitive, see createschema
	PasswordHash string `bun:",notnull" json:"-"`
	DataKey      string `bun:",nullzero" json:"-"`                          // the key of its notes sealed by TIDE_SECRET, see services/secret
	Locale       string `bun:",notnull,default:'fr'" json:"locale"`         // the language of the web app, one of Locales
	Theme        string `bun:",notnull,default:'notes'" json:"theme"`       // the colors of the web app, one of Themes
	ColorMode    string `bun:",notnull,default:'system'" json:"color_mode"` // light or dark, one of ColorModes
	View         string `bun:",notnull,default:'list'" json:"view"`         // how the web app lists items, one of Views
	Sort         string `bun:",notnull,default:'title'" json:"sort"`        // the order of a folder's items, one of Sorts
	Plan         string `bun:",notnull,default:'base'" json:"plan"`         // the features of the web app, one of Plans
	Admin        bool   `bun:",notnull,default:false" json:"admin"`         // may create, invite and delete accounts; the first account of the base gets it
	// Days before the trash is purged, 0 to never purge. An insert of 0 gets the default: only an update sets it.
	TrashRetentionDays int `bun:",notnull,default:30" json:"trash_retention_days"`
	// The most notes sharing a tag that a note shows around it, one of NeighborsLimits, 0 for all of them.
	// An insert of 0 gets the default: only an update sets it.
	NeighborsLimit int           `bun:",notnull,default:50" json:"neighbors_limit"`
	AfterMerge     string        `bun:",notnull,default:'ask'" json:"after_merge"`   // what a merge does with the fork, one of AfterMerges
	Avatar         string        `bun:",type:uuid,nullzero" json:"avatar,omitempty"` // the id of a File of the account, purgefiles keeps it
	Snippets       Snippets      `bun:",type:jsonb,notnull,default:'[]'" json:"snippets"`
	SavedSearches  SavedSearches `bun:",type:jsonb,notnull,default:'[]'" json:"saved_searches"`
	// The order of the sidebar lists and which ones show, [] for the app's default.
	SidebarSections []SidebarSection `bun:",type:jsonb,notnull,default:'[]'" json:"sidebar_sections"`
	CreatedAt       time.Time        `bun:",notnull,default:current_timestamp" json:"created_at"`
	// The version of the settings, bumped by every PATCH /api/me: sent back there, it refuses a write made over an older read.
	UpdatedAt time.Time `bun:",notnull,default:current_timestamp" json:"updated_at"`
	// Bumped by the triggers of createschema at every write that changes a path, from any process: Paths reads it to
	// know its cache is still right. From a sequence, so a rolled-back write never hands its number to another.
	ItemsVersion int64 `bun:",notnull,default:0" json:"-"`
	// The features of its plan, what the web app shows: not stored, filled from Plan at every read.
	Features []string `bun:"-" json:"features"`
}

var _ bun.AfterScanRowHook = (*User)(nil)

// AfterScanRow fills Features, so every answer with the account gives the features of its plan.
func (u *User) AfterScanRow(context.Context) error {
	u.Features = PlanFeatures(u.Plan)
	return nil
}

// Snippet is a piece of markdown of the account, inserted while typing or starting a new note; its #tags become tags of that note.
type Snippet struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Snippets are kept sealed, as one JSON string in their jsonb column: write Seal's.
type Snippets []Snippet

// Seal gives the snippets sealed by the key of the account, as the JSON string to write.
func (s Snippets) Seal(userID string) (string, error) {
	if s == nil {
		s = Snippets{}
	}
	data, err := json.Marshal([]Snippet(s))
	if err != nil {
		return "", err
	}
	sealed, err := secret.Seal(userID, string(data))
	if err != nil {
		return "", err
	}
	data, err = json.Marshal(sealed)
	return string(data), err
}

// Scan also reads a JSON array in clear: the column's default, [].
func (s *Snippets) Scan(src any) error {
	var data []byte
	switch src := src.(type) {
	case string:
		data = []byte(src)
	case []byte:
		data = src
	}
	var sealed string
	if json.Unmarshal(data, &sealed) == nil {
		text, err := secret.Open(sealed)
		if err != nil {
			return err
		}
		data = []byte(text)
	}
	return json.Unmarshal(data, (*[]Snippet)(s))
}

// Locales are the languages of the web app, the first one by default.
var Locales = []string{"fr", "en"}

// Themes, ColorModes, Views and Plans are the other settings of the web app, the first one by default.
// Themes repeats the ids of web/app/utils/themes.ts and Plans the plans of web/app/utils/features.ts, users_test.go checks they match.
var (
	Themes     = []string{"notes", "dracula", "catppuccin", "nord", "gruvbox", "everforest", "tokyo", "vue"}
	ColorModes = []string{"system", "light", "dark"}
	Views      = []string{"list", "grid"}
	// Sorts: by title, creation or last edit, "-" first for the reverse order.
	Sorts = []string{"title", "-title", "created", "-created", "updated", "-updated"}
	Plans = []string{"base", "pro", "ultra"}
	// AfterMerges: the app asks at each merge whether to trash the fork, or trashes it, or keeps it without asking.
	AfterMerges = []string{"ask", "delete", "keep"}
)

// Features are the plan each feature of the web app comes from, each plan including those before it, "off" for no one
// (unfinished, withdrawn): the one list of them. The API refuses the routes of a feature to the plans without it, and
// gives each account its features (User.Features) and each plan its own (GET /api/plans) for the app to show.
var Features = map[string]string{
	"graph":             "base",
	"tabs":              "ultra",
	"splitView":         "ultra",
	"focusMode":         "pro",
	"contextPanel":      "pro",
	"settingsNav":       "pro",
	"slashCommands":     "pro",
	"tables":            "pro",
	"codeHighlight":     "base",
	"callouts":          "pro",
	"wordCount":         "pro",
	"contextMenu":       "pro",
	"paletteActions":    "pro",
	"sourceMode":        "base",
	"noteLock":          "base",
	"noteHistory":       "pro",
	"brokenLinks":       "pro",
	"storageFiles":      "pro",
	"wikiLinks":         "base",
	"linkSuggestions":   "pro",
	"noteEmbeds":        "base",
	"images":            "base",
	"attachments":       "base",
	"linkCards":         "base",
	"tagSuggestions":    "base",
	"snippets":          "base",
	"savedSearches":     "pro",
	"sidebarOrder":      "pro",
	"hideFolders":       "ultra",
	"replaceEverywhere": "ultra",
	"search":            "base",
	"favorites":         "base",
	"backlinks":         "base",
	"themes":            "base",
	"importExport":      "base",
	"trash":             "base",
	"storageUsage":      "base",
}

// Unlocked tells whether an account of plan has feature; an unknown plan has the base features.
func Unlocked(feature, plan string) bool {
	level := slices.Index(Plans, Features[feature])
	return level >= 0 && level <= max(slices.Index(Plans, plan), 0)
}

// PlanFeatures are the features of plan, sorted.
func PlanFeatures(plan string) []string {
	var features []string
	for feature := range Features {
		if Unlocked(feature, plan) {
			features = append(features, feature)
		}
	}
	slices.Sort(features)
	return features
}

// SidebarSection is a list of the web app's sidebar, one of SidebarSectionIDs, shown or hidden.
type SidebarSection struct {
	ID    string `json:"id"`
	Shown bool   `json:"shown"`
}

// SidebarSectionIDs are the lists of the sidebar, in their default order: web/app/utils/sidebar.ts repeats them, users_test.go checks they match.
var SidebarSectionIDs = []string{"favorites", "pinned", "recent", "folders", "savedSearches", "tags"}

// NeighborsLimits are the choices of User.NeighborsLimit, the first one by default; 0 shows them all.
var NeighborsLimits = []int{50, 20, 100, 0}

// Session is a login of a device: the client keeps the token, the base only its hash.
type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:s"`

	ID        string    `bun:",pk,type:uuid,default:gen_random_uuid()"`
	UserID    string    `bun:",notnull,type:uuid"`
	TokenHash string    `bun:",notnull,unique"`
	CreatedAt time.Time `bun:",notnull,default:current_timestamp"`
	ExpiresAt time.Time `bun:",notnull"`
}

// APIToken lets a script or an agent act for the account without a login: notes, folders, tags and files, never
// the account itself (auth refuses it there). The client keeps the token, the base only its hash.
type APIToken struct {
	bun.BaseModel `bun:"table:api_tokens,alias:at" json:"-"`

	ID         string      `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID     string      `bun:",notnull,type:uuid" json:"-"`
	Name       secret.Text `bun:",type:text,notnull" json:"name" swaggertype:"string"` // sealed: a stolen base does not tell what the tokens are for
	TokenHash  string      `bun:",notnull,unique" json:"-"`
	CreatedAt  time.Time   `bun:",notnull,default:current_timestamp" json:"created_at"`
	LastUsedAt time.Time   `bun:",nullzero" json:"last_used_at,omitzero" binding:"optional"` // never used when absent
	ExpiresAt  time.Time   `bun:",nullzero" json:"expires_at,omitzero" binding:"optional"`   // no limit when absent
	// ReadOnly refuses the token every request that changes something (auth), and the MCP its writing tools.
	ReadOnly bool `bun:",notnull,default:false" json:"read_only"`
	// OAuth marks the connection of an OAuth client (the MCP over HTTP): Name is the client's, ExpiresAt the end of its
	// refresh token, 90 days after its last use. TokenHash is then its access token, valid until AccessExpiresAt only.
	OAuth           bool      `bun:"oauth,notnull,default:false" json:"oauth"`
	ClientID        string    `bun:",nullzero" json:"-"`
	RefreshHash     string    `bun:",nullzero" json:"-"`
	AccessExpiresAt time.Time `bun:",nullzero" json:"-"`
}

// OAuthCode is an authorization code of an OAuth client, between the consent and the token: it serves once, and for a
// minute. The client proves with the verifier of CodeChallenge (PKCE) that it is the one which asked.
type OAuthCode struct {
	bun.BaseModel `bun:"table:oauth_codes,alias:oc"`

	CodeHash      string    `bun:",pk"`
	UserID        string    `bun:",notnull,type:uuid"`
	ClientID      string    `bun:",notnull"`
	RedirectURI   string    `bun:",notnull"`
	CodeChallenge string    `bun:",notnull"`
	ReadOnly      bool      `bun:",notnull"`
	ExpiresAt     time.Time `bun:",notnull"`
}

// Throttle counts the wrong tries in a row of a key (a password from an address...): kept in the base, a restart does
// not forget them. ExpiresAt, a day after the last try, lets purgesessions drop it.
type Throttle struct {
	bun.BaseModel `bun:"table:throttles,alias:th"`

	Key       string    `bun:",pk"`
	Count     int       `bun:",notnull"`
	LastAt    time.Time `bun:",notnull"`
	ExpiresAt time.Time `bun:",notnull"`
}

// Invite is a link an admin hands out so someone creates their own account: the base keeps the hash of its token,
// and the token sealed, which only the admin panel reads back.
type Invite struct {
	bun.BaseModel `bun:"table:invites,alias:iv" json:"-"`

	ID        string `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	TokenHash string `bun:",notnull,unique" json:"-"`
	// The token itself, sealed with the key of the admin who made it, so the admin panel shows the link again:
	// "" (NULL) for the invitations made before it was kept. The sign-up still finds an invitation by TokenHash.
	Token     secret.Text `bun:",type:text" json:"token" swaggertype:"string"`
	UserID    string      `bun:",notnull,type:uuid" json:"created_by"` // the admin who made it
	CreatedAt time.Time   `bun:",notnull,default:current_timestamp" json:"created_at"`
	ExpiresAt time.Time   `bun:",notnull" json:"expires_at"`
	UsedAt    time.Time   `bun:",nullzero" json:"-"` // set when it created an account, so a link serves once
}

// AddUserModes are the ways an account is created on a server, the first one by default: by invitation of an admin
// only, or by anyone reaching the web app.
var AddUserModes = []string{"invitation", "public"}

var ErrNoUser = errors.New("not logged in")

type userKey struct{}

// WithUser gives ctx the account the processes act for: the CLI from its session, the API from the request's.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

// UserID is the account of ctx, "" without one. A bun.Tx hands its RunInTx functions the ctx it began with, not the
// one given: begin transactions with a ctx that has the account (the API and the CLI do, tests use testaccount).
func UserID(ctx context.Context) string {
	id, _ := ctx.Value(userKey{}).(string)
	return id
}
