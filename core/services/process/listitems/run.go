package listitems

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

// invalidText is the Postgres SQLSTATE raised when the id is not a uuid.
const invalidText = "22P02"

var (
	ErrInvalidID   = errors.New("not a uuid")
	ErrInvalidKind = errors.New("invalid kind")
	ErrInvalidPage = errors.New("invalid limit or offset")
	ErrInvalidSort = errors.New("unknown sort")
)

type run struct {
	plumprimitive.Process
	cache

	ctx      context.Context
	db       bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	parentID string
	kind     domain.Kind
	limit    int // 0: every child
	offset   int
	sort     string // one of domain.Sorts, "" for title
}

type cache struct {
	parentID string
	order    []string // every child id, sorted
	folders  int      // the folders at the head of order
	result   []domain.Item
	total    int
}

// Process lists the direct children of a folder; an empty parentID lists the root, an empty kind both kinds.
func Process(ctx context.Context, db bun.IDB, parentID string, kind domain.Kind) *run {
	return &run{
		ctx:      ctx,
		db:       db,
		parentID: parentID,
		kind:     kind,
	}
}

// Page keeps limit children from offset, in the same order; GetTotal still counts them all.
func (p *run) Page(limit, offset int) *run {
	p.limit, p.offset = limit, offset
	return p
}

// Sort orders the children by one of domain.Sorts, folders first then the pinned notes and favorite folders.
func (p *run) Sort(sort string) *run {
	p.sort = sort
	return p
}

func (p *run) Run() {
	p.checkKind()
	p.checkPage()
	p.checkSort()
	p.makeParentID()
	p.loadOrder()
	p.loadPinned()
	p.loadItems()
}

// Getters & Setters

func (p *run) GetResult() []domain.Item {
	return p.cache.result
}

func (p *run) GetTotal() int {
	return p.cache.total
}

// Methods

func (p *run) checkKind() {
	if p.kind != "" && p.kind != domain.FOLDER && p.kind != domain.NOTE {
		p.SetErr(fmt.Errorf("%w: %s", ErrInvalidKind, p.kind))
	}
}

func (p *run) checkPage() {
	if p.limit < 0 || p.offset < 0 {
		p.SetErr(fmt.Errorf("%w: %d, %d", ErrInvalidPage, p.limit, p.offset))
	}
}

func (p *run) checkSort() {
	if p.sort != "" && !slices.Contains(domain.Sorts, p.sort) {
		p.SetErr(fmt.Errorf("%w: %s", ErrInvalidSort, p.sort))
	}
}

func (p *run) makeParentID() {
	p.cache.parentID = strings.TrimSpace(p.parentID)
}

// loadOrder sorts the children folders first, then notes; soft_delete hides the trashed ones.
func (p *run) loadOrder() {
	if p.HasErr() {
		return
	}
	if by := strings.TrimPrefix(p.sort, "-"); by == "created" || by == "updated" {
		p.loadDateOrder(by)
		return
	}
	p.loadTitleOrder()
}

// loadDateOrder sorts by date in SQL: an edit moves updated_at without bumping users.items_version, nothing cached.
func (p *run) loadDateOrder(by string) {
	column := "created_at"
	if by == "updated" {
		column = "coalesce(updated_at, created_at)" // a note never edited has none
	}
	direction := "ASC"
	if strings.HasPrefix(p.sort, "-") {
		direction = "DESC"
	}
	var children []domain.Item
	query := p.db.NewSelect().Model(&children).Column("id", "kind").
		OrderExpr("kind = ? DESC, "+column+" "+direction+", id", domain.FOLDER)
	if err := p.children(query).Scan(p.ctx); err != nil {
		p.SetErr(toDomainErr(err))
		return
	}
	p.cache.order = make([]string, len(children))
	for i, item := range children {
		p.cache.order[i] = item.ID
		if item.Kind == domain.FOLDER {
			p.cache.folders++
		}
	}
}

// loadTitleOrder sorts by title (sealed: ordered here). The titles come from the cached paths (none decrypted), and
// the order itself is cached while users.items_version stays the same: sorting 48,000 notes takes 140 ms, redone on
// every page otherwise. "-title" reads it backwards in loadPinned.
func (p *run) loadTitleOrder() {

	account, err := orders.Load(p.ctx, p.db, func() (*cachedOrders, error) {
		return &cachedOrders{ids: map[orderKey]cachedOrder{}}, nil
	})
	if err != nil {
		p.SetErr(err)
		return
	}
	key := orderKey{p.cache.parentID, p.kind}
	account.mu.Lock()
	order, ok := account.ids[key]
	account.mu.Unlock()
	if ok {
		p.cache.order, p.cache.folders = order.ids, order.folders
		return
	}

	// By id under the same title: the order stays the same from one page to the next.
	var children []domain.Item
	query := p.db.NewSelect().Model(&children).Column("id", "kind").OrderExpr("kind = ? DESC, id", domain.FOLDER)
	if err := p.children(query).Scan(p.ctx); err != nil {
		p.SetErr(toDomainErr(err))
		return
	}
	paths, err := domain.Paths(p.ctx, p.db)
	if err != nil {
		p.SetErr(err)
		return
	}
	// A child's path is its parent's, then "/" and its title; the root's is "".
	prefix := len(paths[p.cache.parentID]) + 1
	domain.SortFold(children, func(i domain.Item) string {
		if path := paths[i.ID]; len(path) >= prefix {
			return path[prefix:]
		}
		return "" // under a trashed folder: no path
	}, func(a, b domain.Item) int {
		return boolOrder(b.Kind == domain.FOLDER) - boolOrder(a.Kind == domain.FOLDER)
	})

	p.cache.order = make([]string, len(children))
	for i, item := range children {
		p.cache.order[i] = item.ID
		if item.Kind == domain.FOLDER {
			p.cache.folders++
		}
	}
	account.mu.Lock()
	account.ids[key] = cachedOrder{p.cache.order, p.cache.folders}
	account.mu.Unlock()
}

// loadPinned puts the pinned notes and favorite folders first among their kind, read at every call: pinning does
// not bump users.items_version. "-title" reverses each kind here, the cached order shared and left as it is.
func (p *run) loadPinned() {
	if p.HasErr() {
		return
	}

	var ids []string
	query := p.db.NewSelect().Model((*domain.Item)(nil)).Column("id").
		Where("EXISTS (SELECT 1 FROM metadata_notes AS mn WHERE mn.item_id = i.id AND mn.pinned) OR " +
			"EXISTS (SELECT 1 FROM metadata_folders AS mf WHERE mf.item_id = i.id AND mf.pinned)")
	if err := p.children(query).Scan(p.ctx, &ids); err != nil {
		p.SetErr(err)
		return
	}
	p.cache.order = arrange(p.cache.order, p.cache.folders, p.sort == "-title", ids)
}

// loadItems reads only the page of the order: a folder may hold 48,000 notes. readonly marks the locked notes,
// for their padlock in the folder; has_folders tells the sidebar a folder opens without reading its children.
func (p *run) loadItems() {
	if p.HasErr() {
		return
	}

	order := p.cache.order
	p.cache.total = len(order)
	paged := p.limit > 0
	if paged {
		order = order[min(p.offset, len(order)):min(p.offset+p.limit, len(order))]
	}
	if len(order) == 0 {
		return
	}

	var items []domain.Item
	query := p.children(p.db.NewSelect().Model(&items)).
		Column("id", "parent_id", "created_at", "updated_at", "kind", "title").
		ColumnExpr("COALESCE((SELECT mn.readonly FROM metadata_notes AS mn WHERE mn.item_id = i.id), false) AS readonly").
		ColumnExpr("EXISTS (SELECT 1 FROM items AS c WHERE c.parent_id = i.id AND c.kind = ? AND c.deleted_at IS NULL) AS has_folders", domain.FOLDER)
	if paged {
		query.Where("id IN (?)", bun.In(order))
	}
	if err := query.Scan(p.ctx); err != nil {
		p.SetErr(err)
		return
	}

	byID := make(map[string]domain.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	p.cache.result = make([]domain.Item, 0, len(order))
	for _, id := range order {
		if full, ok := byID[id]; ok { // trashed between the two reads
			p.cache.result = append(p.cache.result, full)
		}
	}
}

// children filters a query on items to the asked children of the account.
func (p *run) children(query *bun.SelectQuery) *bun.SelectQuery {
	query.Where("user_id = ?", domain.UserID(p.ctx))
	if p.cache.parentID == "" {
		query.Where("parent_id IS NULL")
	} else {
		query.Where("parent_id = ?", p.cache.parentID)
	}
	if p.kind != "" {
		query.Where("kind = ?", p.kind)
	}
	return query
}

// orders keeps, by account, the sorted children ids of the folders listed since its items last changed: a change
// drops them all, so an account holds at most about twice its items. Ids only, no title.
var orders domain.Versioned[*cachedOrders]

type cachedOrders struct {
	mu  sync.Mutex
	ids map[orderKey]cachedOrder
}

type cachedOrder struct {
	ids     []string // shared, do not modify
	folders int
}

type orderKey struct {
	parentID string
	kind     domain.Kind
}

// Helpers

// arrange gives the order in a new slice: in each kind, reversed if asked, the pinned ids first.
func arrange(order []string, folders int, reverse bool, pinned []string) []string {
	first := make(map[string]bool, len(pinned))
	for _, id := range pinned {
		first[id] = true
	}
	result := make([]string, 0, len(order))
	for _, kind := range [][]string{order[:folders], order[folders:]} {
		if reverse {
			kind = slices.Clone(kind)
			slices.Reverse(kind)
		}
		for _, wanted := range []bool{true, false} {
			for _, id := range kind {
				if first[id] == wanted {
					result = append(result, id)
				}
			}
		}
	}
	return result
}

// toDomainErr turns a parent id that is not a uuid into ErrInvalidID.
func toDomainErr(err error) error {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) && pgErr.Field('C') == invalidText {
		return fmt.Errorf("%w: %s", ErrInvalidID, pgErr.Field('M'))
	}
	return err
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}
