package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moodiness/polyfin/internal/testdb"
)

// quiet logs nothing, for the tests that do not look at the logs.
var quiet = slog.New(slog.DiscardHandler)

// migrate applies migrations as Migrate does, logging nothing.
func migrate(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	return migrator{logger: quiet}.migrate(ctx, pool, migrations)
}

// migrateShipped applies the migrations this build ships, logging
// nothing.
func migrateShipped(ctx context.Context, pool *pgxpool.Pool) error {
	return Migrate(ctx, pool, quiet)
}

// records is a slog.Handler keeping every record, whose waitFor waits for
// one: the tests wait on what Migrate logs, never on time.
type records struct {
	mu      sync.Mutex
	kept    []slog.Record
	changed chan struct{}
}

func newRecords() *records { return &records{changed: make(chan struct{}, 1)} }

func (r *records) Enabled(context.Context, slog.Level) bool { return true }
func (r *records) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *records) WithGroup(string) slog.Handler            { return r }

func (r *records) Handle(_ context.Context, record slog.Record) error {
	r.mu.Lock()
	r.kept = append(r.kept, record.Clone())
	r.mu.Unlock()
	select {
	case r.changed <- struct{}{}:
	default:
	}
	return nil
}

// find is the first record of message, with its attributes by key.
func (r *records) find(message string) (slog.Record, map[string]slog.Value, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.kept {
		if record.Message != message {
			continue
		}
		attrs := map[string]slog.Value{}
		record.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value
			return true
		})
		return record, attrs, true
	}
	return slog.Record{}, nil, false
}

// waitFor waits for a record of message, with no time limit of its own:
// one never logged fails the test at go test's timeout.
func (r *records) waitFor(message string) (slog.Record, map[string]slog.Value) {
	for {
		if record, attrs, ok := r.find(message); ok {
			return record, attrs
		}
		<-r.changed
	}
}

// While another session, as another Polyfin would, holds the migration
// lock, Migrate waits and says so, naming that session; it migrates once
// the lock is let go, and logs the migration with its duration.
func TestMigrateWaitsForAnotherPolyfinAndSaysSo(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	other, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	if _, err := other.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		t.Fatal(err)
	}
	log := newRecords()
	done := make(chan error, 1)
	go func() {
		done <- migrator{logger: slog.New(log), poll: time.Millisecond}.migrate(ctx, pool, []migration{step(1, "a", "CREATE TABLE a (x int)")})
	}()
	_, attrs := log.waitFor("Waiting for another Polyfin to finish migrating")
	if holder := attrs["holder_pid"]; holder.Kind() != slog.KindInt64 || uint32(holder.Int64()) != other.Conn().PgConn().PID() {
		t.Errorf("holder %v, not the session holding the lock, %d", holder, other.Conn().PgConn().PID())
	}
	select {
	case err := <-done:
		t.Fatalf("migrated while another session held the lock: %v", err)
	default:
	}
	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLock); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	log.waitFor("Another Polyfin finished migrating")
	// A database being created logs each migration at Debug.
	record, attrs := log.waitFor("Applied a database migration")
	if record.Level != slog.LevelDebug || attrs["migration"].String() != "0001_a" || attrs["duration"].Kind() != slog.KindDuration {
		t.Errorf("logged %v: %v", record.Level, attrs)
	}
	if _, attrs := log.waitFor("The database is migrated"); attrs["migrations"].Int64() != 1 {
		t.Errorf("logged %v", attrs)
	}
}

// A migration waiting on a lock another session holds says which session
// it waits for, never its statement, and goes on once the lock is let
// go; an upgrade logs each migration at Info.
func TestAMigrationWaitingOnALockNamesTheSession(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	first := []migration{step(1, "a", "CREATE TABLE a (x int)")}
	if err := migrate(ctx, pool, first); err != nil {
		t.Fatal(err)
	}
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := other.Exec(ctx, "SELECT x, 'the statement' FROM a"); err != nil {
		t.Fatal(err)
	}
	log := newRecords()
	done := make(chan error, 1)
	go func() {
		done <- migrator{logger: slog.New(log), every: time.Millisecond, blockedAfter: time.Millisecond}.migrate(ctx, pool,
			append(first, step(2, "b", "ALTER TABLE a ADD COLUMN y int")))
	}()
	record, attrs := log.waitFor("A database migration waits for locks other sessions hold")
	var blocking []string
	if sessions, ok := attrs["blocking"].Any().([]string); ok {
		blocking = sessions
	}
	pid := other.Conn().PgConn().PID()
	if record.Level != slog.LevelWarn || attrs["migration"].String() != "0002_b" || len(blocking) != 1 ||
		!strings.HasPrefix(blocking[0], fmt.Sprintf("pid %d, ", pid)) || !strings.Contains(blocking[0], "idle in transaction") ||
		strings.Contains(blocking[0], "the statement") {
		t.Errorf("logged %v %v", record.Level, attrs)
	}
	if err := other.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if record, attrs := log.waitFor("Applied a database migration"); record.Level != slog.LevelInfo || attrs["migration"].String() != "0002_b" {
		t.Errorf("logged %v %v", record.Level, attrs)
	}
}
