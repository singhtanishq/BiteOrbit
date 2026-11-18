// Package migrations applies ordered SQL migrations embedded in the binary.
// Each migration runs in its own transaction and is recorded in schema_migrations.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed *.sql
var migrationFS embed.FS

// Migration is one ordered SQL file.
type Migration struct {
	Name string
	SQL  string
}

// All returns migrations sorted by filename (NNNN_name.sql convention).
func All() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]Migration, 0, len(names))
	for _, name := range names {
		data, err := migrationFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		out = append(out, Migration{Name: name, SQL: string(data)})
	}
	return out, nil
}

// Runner applies migrations using a *sql.DB (stdlib driver via pgx stdlib adapter).
type Runner struct {
	DB *sql.DB
}

func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name        text PRIMARY KEY,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)`)
	return err
}

// Status lists pending and applied migrations.
func (r *Runner) Status(ctx context.Context) (applied []string, pending []string, err error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, nil, err
		}
		applied = append(applied, name)
	}
	appliedSet := map[string]bool{}
	for _, a := range applied {
		appliedSet[a] = true
	}
	all, err := All()
	if err != nil {
		return nil, nil, err
	}
	for _, m := range all {
		if !appliedSet[m.Name] {
			pending = append(pending, m.Name)
		}
	}
	return applied, pending, nil
}

// Up applies all pending migrations in order. Idempotent; safe to re-run.
func (r *Runner) Up(ctx context.Context) (applied []string, err error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, err
	}
	all, err := All()
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		var exists bool
		if err := r.DB.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, m.Name).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		tx, err := r.DB.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %s failed: %w", m.Name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES ($1, $2)`, m.Name, time.Now()); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("record migration %s: %w", m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return applied, fmt.Errorf("commit migration %s: %w", m.Name, err)
		}
		applied = append(applied, m.Name)
	}
	return applied, nil
}
