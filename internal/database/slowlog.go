package database

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// slowStatement is how long a statement runs before the log tells it:
// past what a page of the admin app or of a player waits for one
// statement, and short of the 5 seconds a whole listing is logged at.
const slowStatement = 500 * time.Millisecond

// The log of slow statements writes at most slowBurst lines at once, then
// one more each slowRefill: a database slow for every statement does not
// flood the log. It counts the lines left out, and the next line says
// how many.
const (
	slowBurst  = 20
	slowRefill = time.Second
)

// slowSQL bounds the characters of a slow statement's SQL in the log.
const slowSQL = 300

// slowLog is the pool's tracer: it logs the statements, and the queries
// of batches, that run for slowStatement or longer, at Info, with how
// long they ran and their SQL, its spaces collapsed and cut at slowSQL
// characters. It never logs the arguments, which can hold secrets. A
// statement that runs faster costs two readings of the clock and the
// context value that carries its start; nothing is formatted.
type slowLog struct {
	logger    *slog.Logger
	threshold time.Duration
	// now stands for time.Now, for tests.
	now func() time.Time

	mu      sync.Mutex
	tokens  float64
	filled  time.Time
	skipped int
}

// newSlowLog is a slowLog logging into logger.
func newSlowLog(logger *slog.Logger) *slowLog {
	return &slowLog{logger: logger, threshold: slowStatement, now: time.Now, tokens: slowBurst}
}

// started is when a statement started, and its SQL, as the context of
// the statement carries them.
type started struct {
	at  time.Time
	sql string
}

// startedKey is the context key of started.
type startedKey struct{}

func (s *slowLog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, startedKey{}, &started{at: s.now(), sql: data.SQL})
}

// TraceQueryEnd is called once the statement's rows are read and closed,
// so that its time includes the caller's reading of them.
func (s *slowLog) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(startedKey{}).(*started)
	if !ok {
		return
	}
	if took := s.now().Sub(start.at); took >= s.threshold {
		s.log(ctx, took, start.sql, data.Err)
	}
}

func (s *slowLog) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}

// TraceBatchQuery times each query of a batch from when its result was
// asked for, as pgx tells.
func (s *slowLog) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	if data.ReadStartTime.IsZero() {
		return
	}
	if took := s.now().Sub(data.ReadStartTime); took >= s.threshold {
		s.log(ctx, took, data.SQL, data.Err)
	}
}

func (s *slowLog) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// log writes a slow statement's line, unless the lines of the moment are
// spent (see slowBurst).
func (s *slowLog) log(ctx context.Context, took time.Duration, sql string, err error) {
	s.mu.Lock()
	now := s.now()
	if !s.filled.IsZero() {
		s.tokens = min(slowBurst, s.tokens+float64(now.Sub(s.filled))/float64(slowRefill))
	}
	s.filled = now
	if s.tokens < 1 {
		s.skipped++
		s.mu.Unlock()
		return
	}
	s.tokens--
	skipped := s.skipped
	s.skipped = 0
	s.mu.Unlock()
	attrs := []any{"duration", took.Round(time.Millisecond), "sql", shortSQL(sql)}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	if skipped > 0 {
		attrs = append(attrs, "skipped", skipped)
	}
	s.logger.InfoContext(ctx, "A database statement was slow", attrs...)
}

// shortSQL is sql on one line, its runs of spaces and line breaks made
// one space, cut at slowSQL characters with an ellipsis.
func shortSQL(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if utf8.RuneCountInString(sql) <= slowSQL {
		return sql
	}
	cut, n := 0, 0
	for i := range sql {
		if n == slowSQL {
			cut = i
			break
		}
		n++
	}
	return sql[:cut] + "…"
}
