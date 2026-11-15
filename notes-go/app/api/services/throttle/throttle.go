// Package throttle makes a key wait longer after each try in a row: wrong passwords at the login and on the OAuth
// consent page, accounts created from one address at the sign-up. The tries are kept in the base (throttles), so a
// restart forgets none: the consent page faces the Internet.
package throttle

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/api/services/respond"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// waits is how long to wait after 1, 2, 3... tries in a row, like an iPhone: a password cannot be guessed by trying them all.
var waits = []time.Duration{0, 0, 0, 30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

// forget is how long a key keeps its tries after the last one: its next try starts over, purgesessions drops the row.
const forget = 24 * time.Hour

// Throttle counts the tries in a row of each key, under its name.
type Throttle struct {
	db   bun.IDB
	name string
	now  func() time.Time
	// message is the error of the 429.
	message string
}

// New gives a throttle whose keys are counted under name, and whose 429 says message.
func New(db bun.IDB, name, message string) *Throttle {
	return &Throttle{db: db, name: name, now: time.Now, message: message}
}

// Passwords counts the wrong passwords: the login and the OAuth consent page share the count of an address.
func Passwords(db bun.IDB) *Throttle {
	return New(db, "password", "too many wrong passwords, try again later")
}

// Try counts a try for key before it runs, and gives 0; or, while key waits, how long it has left, without counting.
// One statement: tries sent at once cannot all pass while argon2id runs. The caller resets on a good password,
// forgives on another error.
func (t *Throttle) Try(ctx context.Context, key string) (time.Duration, error) {
	now := t.now()
	seconds := make([]float64, len(waits))
	for i, wait := range waits {
		seconds[i] = wait.Seconds()
	}
	var count int
	err := t.db.NewRaw(`
		INSERT INTO throttles AS th (key, count, last_at, expires_at) VALUES (?, 1, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			count = CASE WHEN th.expires_at <= excluded.last_at THEN 1 ELSE th.count + 1 END,
			last_at = excluded.last_at, expires_at = excluded.expires_at
		WHERE th.expires_at <= excluded.last_at
			OR th.last_at + make_interval(secs => (?::float8[])[least(th.count, ?)]) <= excluded.last_at
		RETURNING count`,
		t.key(key), now, now.Add(forget), pgdialect.Array(seconds), len(waits)).Scan(ctx, &count)
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	var strikes domain.Throttle
	if err := t.db.NewSelect().Model(&strikes).Where("key = ?", t.key(key)).Scan(ctx); err != nil {
		return 0, err
	}
	// Never 0 here: the try was refused, the client must wait at least a second.
	return max(waits[min(strikes.Count, len(waits))-1]-now.Sub(strikes.LastAt), time.Second), nil
}

// Forgive takes back the try Try counted, when it did not count.
func (t *Throttle) Forgive(ctx context.Context, key string) {
	_, err := t.db.NewRaw(`
		WITH gone AS (DELETE FROM throttles WHERE key = ? AND count <= 1)
		UPDATE throttles SET count = count - 1 WHERE key = ? AND count > 1`, t.key(key), t.key(key)).Exec(ctx)
	if err != nil {
		log.Printf("throttle: forgive: %v", err)
	}
}

// Reset forgets the tries of key.
func (t *Throttle) Reset(ctx context.Context, key string) {
	if _, err := t.db.NewDelete().Model((*domain.Throttle)(nil)).Where("key = ?", t.key(key)).Exec(ctx); err != nil {
		log.Printf("throttle: reset: %v", err)
	}
}

// Blocked answers 429 with Retry-After while key waits, else counts the try (Try).
func (t *Throttle) Blocked(w http.ResponseWriter, r *http.Request, key string) bool {
	wait, err := t.Try(r.Context(), key)
	if err != nil {
		respond.Error(w, err)
		return true
	}
	if wait == 0 {
		return false
	}
	w.Header().Set("Retry-After", RetryAfter(wait))
	respond.JSON(w, http.StatusTooManyRequests, respond.Failure{Error: t.message})
	return true
}

// RetryAfter is wait in whole seconds, rounded up, for the Retry-After header.
func RetryAfter(wait time.Duration) string {
	return strconv.Itoa(int(math.Ceil(wait.Seconds())))
}

func (t *Throttle) key(key string) string {
	return t.name + " " + key
}
