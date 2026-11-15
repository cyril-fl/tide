package throttle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/uptrace/bun"
)

func connect(t *testing.T) *bun.DB {
	t.Helper()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	t.Cleanup(func() { db.Close() })
	schema := createschema.Process(context.Background(), db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	return db
}

func TestThrottle(t *testing.T) {
	ctx := context.Background()
	tx, err := connect(t).BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	now := time.Now().Truncate(time.Second)
	th := New(tx, "test", "slow down")
	th.now = func() time.Time { return now }
	try := func() time.Duration {
		t.Helper()
		wait, err := th.Try(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		return wait
	}

	want := []time.Duration{0, 0, 0, 30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour}
	try()
	for i, wait := range want[:len(want)-1] {
		if wait > 0 {
			if got := try(); got != wait {
				t.Errorf("after %d tries: wait = %v, want %v", i+1, got, wait)
			}
			now = now.Add(wait)
		}
		if got := try(); got != 0 {
			t.Errorf("after %d tries and the wait: wait = %v, want 0", i+1, got)
		}
	}

	// Another name or key counts apart.
	if wait, err := New(tx, "other", "").Try(ctx, "a"); err != nil || wait != 0 {
		t.Errorf("other name: wait = %v, %v, want 0", wait, err)
	}
	if wait, err := th.Try(ctx, "b"); err != nil || wait != 0 {
		t.Errorf("other key: wait = %v, %v, want 0", wait, err)
	}

	now = now.Add(10 * time.Minute)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if !th.Blocked(w, r, "a") || w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "3000" {
		t.Errorf("blocked: status = %d, Retry-After = %q, want 429 and 3000", w.Code, w.Header().Get("Retry-After"))
	}

	// A restart keeps the tries: a new throttle on the same base still waits.
	again := New(tx, "test", "")
	again.now = th.now
	if wait, err := again.Try(ctx, "a"); err != nil || wait != 50*time.Minute {
		t.Errorf("after a restart: wait = %v, %v, want 50m", wait, err)
	}

	// A day after the last try, the count starts over.
	now = now.Add(forget)
	if got := try(); got != 0 {
		t.Errorf("a day later: wait = %v, want 0", got)
	}
	if got := try(); got != 0 {
		t.Errorf("second try a day later: wait = %v, want 0", got)
	}

	th.Reset(ctx, "a")
	try()
	var count int
	if err := tx.NewSelect().Model((*domain.Throttle)(nil)).Column("count").Where("key = ?", "test a").Scan(ctx, &count); err != nil || count != 1 {
		t.Errorf("first try after a reset: count = %d, %v, want 1", count, err)
	}
	th.Forgive(ctx, "a")
	if exists, err := tx.NewSelect().Model((*domain.Throttle)(nil)).Where("key = ?", "test a").Exists(ctx); err != nil || exists {
		t.Errorf("forgiven down to 0: row kept = %v, %v, want none", exists, err)
	}
}

func TestThrottleAtOnce(t *testing.T) {
	ctx := context.Background()
	db := connect(t)
	th := New(db, "test-at-once", "")
	key := time.Now().String()
	t.Cleanup(func() { th.Reset(ctx, key) })

	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := 0
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait, err := th.Try(ctx, key)
			if err != nil {
				t.Error(err)
			}
			if wait == 0 {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if passed != 4 {
		t.Errorf("tries sent at once: %d passed, want 4", passed)
	}

	th.Forgive(ctx, key)
	var count int
	if err := db.NewSelect().Model((*domain.Throttle)(nil)).Column("count").Where("key = ?", "test-at-once "+key).Scan(ctx, &count); err != nil || count != 3 {
		t.Errorf("after forgive: count = %d, %v, want 3", count, err)
	}
}
