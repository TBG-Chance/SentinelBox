package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/TBG-Chance/SentinelBox/internal/config"
	"github.com/TBG-Chance/SentinelBox/migrations"
	_ "modernc.org/sqlite"
)

type Store struct {
	db            *sql.DB
	schemaVersion int
}

func Open(ctx context.Context, cfg config.DatabaseConfig) (*Store, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}

	path, err := filepath.Abs(strings.TrimSpace(cfg.Path))
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	directory := filepath.Dir(path)
	if err := ensureDirectory(directory); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dataSourceName(path, cfg.BusyTimeout))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConnections)
	db.SetMaxIdleConns(cfg.MaxOpenConnections)

	closeOnError := func(failure error) (*Store, error) {
		_ = db.Close()
		return nil, failure
	}
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("ping SQLite database: %w", err))
	}
	if err := os.Chmod(path, 0o640); err != nil {
		return closeOnError(fmt.Errorf("protect database file: %w", err))
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		return closeOnError(fmt.Errorf("enable SQLite WAL mode: %w", err))
	}
	if !strings.EqualFold(journalMode, "wal") {
		return closeOnError(fmt.Errorf("SQLite journal mode is %q; expected wal", journalMode))
	}

	version, err := applyMigrations(ctx, db, migrations.Files)
	if err != nil {
		return closeOnError(err)
	}
	store := &Store{db: db, schemaVersion: version}
	if err := store.verifyPragmas(ctx, cfg); err != nil {
		return closeOnError(err)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Check(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	if err := s.db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("database ping: %w", err)
	}
	version, err := currentSchemaVersion(ctx, s.db)
	if err != nil {
		return 0, err
	}
	if version != s.schemaVersion {
		return version, fmt.Errorf("database schema changed from %d to %d after startup", s.schemaVersion, version)
	}
	return version, nil
}

func (s *Store) verifyPragmas(ctx context.Context, cfg config.DatabaseConfig) error {
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify SQLite foreign keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("SQLite foreign keys are disabled")
	}

	var synchronous int
	if err := s.db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return fmt.Errorf("verify SQLite synchronous mode: %w", err)
	}
	if synchronous != 1 {
		return fmt.Errorf("SQLite synchronous mode is %d; expected NORMAL (1)", synchronous)
	}

	var busyTimeout int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify SQLite busy timeout: %w", err)
	}
	if busyTimeout != cfg.BusyTimeout.Milliseconds() {
		return fmt.Errorf("SQLite busy timeout is %dms; expected %dms", busyTimeout, cfg.BusyTimeout.Milliseconds())
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("database directory path is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect database directory: %w", err)
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func dataSourceName(path string, busyTimeout config.Duration) string {
	slashPath := filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); volume != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	u := url.URL{Scheme: "file", Path: slashPath}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(ON)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	query.Add("_pragma", "synchronous(NORMAL)")
	u.RawQuery = query.Encode()
	return u.String()
}
