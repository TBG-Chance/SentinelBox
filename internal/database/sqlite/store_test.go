package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

func TestOpenInitializesAndReopensDatabase(t *testing.T) {
	cfg := testDatabaseConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open(first): %v", err)
	}
	version, err := store.Check(ctx)
	if err != nil {
		t.Fatalf("Check(first): %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}
	for _, table := range []string{"application_metadata", "schema_migrations"} {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d, want 1", table, count)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open(second): %v", err)
	}
	defer reopened.Close()
	if version, err := reopened.Check(ctx); err != nil || version != 1 {
		t.Fatalf("Check(second) = (%d, %v), want (1, nil)", version, err)
	}
}

func TestOpenRejectsChangedAppliedMigration(t *testing.T) {
	cfg := testDatabaseConfig(t)
	ctx := context.Background()
	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'changed' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, cfg)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Open() error = %v, want migration mismatch", err)
	}
}

func TestOpenRejectsUnknownDatabaseVersion(t *testing.T) {
	cfg := testDatabaseConfig(t)
	ctx := context.Background()
	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, checksum, applied_at_ms) VALUES (99, 'future.up.sql', 'future', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown migration version 99") {
		t.Fatalf("Open() error = %v, want unknown-version error", err)
	}
}

func TestCheckFailsAfterClose(t *testing.T) {
	store, err := Open(context.Background(), testDatabaseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Check(context.Background()); err == nil {
		t.Fatal("Check succeeded after Close")
	}
}

func TestDataSourceNameEscapesPathAndIncludesPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database with space.db")
	dsn := dataSourceName(path, config.NewDuration(5*time.Second))
	for _, expected := range []string{"file:", "database%20with%20space.db", "busy_timeout%285000%29", "foreign_keys%28ON%29", "synchronous%28NORMAL%29"} {
		if !strings.Contains(dsn, expected) {
			t.Fatalf("DSN %q does not contain %q", dsn, expected)
		}
	}
}

func testDatabaseConfig(t *testing.T) config.DatabaseConfig {
	t.Helper()
	return config.DatabaseConfig{
		Path:               filepath.Join(t.TempDir(), "data", "sentinel.db"),
		BusyTimeout:        config.NewDuration(5 * time.Second),
		MaxOpenConnections: 4,
	}
}
