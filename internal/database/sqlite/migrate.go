package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const createMigrationTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    checksum TEXT NOT NULL,
    applied_at_ms INTEGER NOT NULL
) STRICT;
`

type migration struct {
	version  int
	name     string
	checksum string
	sql      string
}

type appliedMigration struct {
	name     string
	checksum string
}

func applyMigrations(ctx context.Context, db *sql.DB, files embed.FS) (int, error) {
	migrations, err := loadMigrations(files)
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, fmt.Errorf("no embedded database migrations were found")
	}

	if _, err := db.ExecContext(ctx, createMigrationTableSQL); err != nil {
		return 0, fmt.Errorf("create migration table: %w", err)
	}

	applied, err := loadAppliedMigrations(ctx, db)
	if err != nil {
		return 0, err
	}

	available := make(map[int]migration, len(migrations))
	for _, item := range migrations {
		available[item.version] = item
	}
	for version, record := range applied {
		item, ok := available[version]
		if !ok {
			return 0, fmt.Errorf("database contains unknown migration version %d", version)
		}
		if record.name != item.name || record.checksum != item.checksum {
			return 0, fmt.Errorf("migration %03d does not match the embedded migration", version)
		}
	}

	for _, item := range migrations {
		if _, ok := applied[item.version]; ok {
			continue
		}
		if err := applyMigration(ctx, db, item); err != nil {
			return 0, err
		}
	}

	return migrations[len(migrations)-1].version, nil
}

func loadMigrations(files embed.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	result := make([]migration, 0, len(entries))
	seenVersions := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration filename %q must start with a numeric version", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration filename %q has an invalid version", entry.Name())
		}
		if existing, exists := seenVersions[version]; exists {
			return nil, fmt.Errorf("migrations %q and %q share version %d", existing, entry.Name(), version)
		}
		contents, err := files.ReadFile(filepath.ToSlash(entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		sum := sha256.Sum256(contents)
		result = append(result, migration{
			version:  version,
			name:     entry.Name(),
			checksum: hex.EncodeToString(sum[:]),
			sql:      string(contents),
		})
		seenVersions[version] = entry.Name()
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].version < result[j].version
	})
	for index, item := range result {
		expected := index + 1
		if item.version != expected {
			return nil, fmt.Errorf("migration sequence has version %d; expected %d", item.version, expected)
		}
	}
	return result, nil
}

func loadAppliedMigrations(ctx context.Context, db *sql.DB) (map[int]appliedMigration, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	result := make(map[int]appliedMigration)
	for rows.Next() {
		var version int
		var record appliedMigration
		if err := rows.Scan(&version, &record.name, &record.checksum); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		result[version] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return result, nil
}

func applyMigration(ctx context.Context, db *sql.DB, item migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %03d: %w", item.version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		return fmt.Errorf("apply migration %03d: %w", item.version, err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name, checksum, applied_at_ms) VALUES (?, ?, ?, ?)`,
		item.version,
		item.name,
		item.checksum,
		time.Now().UTC().UnixMilli(),
	); err != nil {
		return fmt.Errorf("record migration %03d: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %03d: %w", item.version, err)
	}
	return nil
}

func currentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
