---
name: new-migration
description: Create a new database migration pair (up + down SQL files)
---

# New Database Migration

## When to Use

Use this skill when adding, modifying, or removing database tables, columns, indexes, or views.

## Prerequisites

- Understand what schema change is needed
- Know whether it's additive (new table/column) or destructive (drop/rename)

## Steps

### Step 1: Determine the Next Migration Number

Check the highest existing migration number:

```bash
ls internal/adapters/storage/sqlite/migrations/*.up.sql | sort | tail -1
```

The new migration number is one higher, zero-padded to 6 digits (e.g., if highest is `000053`, next is `000054`).

### Step 2: Create the Migration Pair

Create both `.up.sql` and `.down.sql` files:

```bash
touch internal/adapters/storage/sqlite/migrations/000054_description.up.sql
touch internal/adapters/storage/sqlite/migrations/000054_description.down.sql
```

**Naming convention:** `NNNNNN_short_snake_case_description.{up,down}.sql`

### Step 3: Write the Up Migration

Write the forward migration SQL. Guidelines:

- Use `IF NOT EXISTS` for CREATE TABLE/INDEX
- All monetary values stored as INTEGER (cents)
- Include appropriate indexes for query patterns
- Add CHECK constraints where applicable
- Use `TEXT` for strings, `INTEGER` for numbers/booleans/timestamps, `REAL` for floating point

Common patterns:

```sql
-- New table
CREATE TABLE IF NOT EXISTS example (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- New column
ALTER TABLE existing_table ADD COLUMN new_col TEXT NOT NULL DEFAULT '';

-- New index
CREATE INDEX IF NOT EXISTS idx_table_column ON table_name(column_name);
```

### Step 4: Write the Down Migration

Write the reverse migration SQL. This MUST undo exactly what the up migration did:

- `CREATE TABLE` → `DROP TABLE IF EXISTS`
- `ALTER TABLE ADD COLUMN` → `ALTER TABLE DROP COLUMN` (SQLite 3.35+)
- `CREATE INDEX` → `DROP INDEX IF EXISTS`

**Important:** If the down migration cannot safely reverse the change (e.g., data loss from dropping a populated column), add a comment explaining the risk.

### Step 5: Verify the Migration Embeds

Migrations are embedded via Go's `embed.FS` in `internal/adapters/storage/sqlite/migrations.go`. No code change is needed — the `//go:embed migrations/*.sql` directive picks up new files automatically.

Verify the project builds:

```bash
go build ./...
```

### Step 6: Update Documentation

1. Update `docs/SCHEMA.md` with the new table/column/index
2. Update the migration count in `CLAUDE.md` (Database section) — change the count and the range end

### Step 7: Test

Run the storage tests to ensure no migration conflicts:

```bash
go test ./internal/adapters/storage/sqlite/... -v
```

## Checklist

- [ ] Migration number is sequential (no gaps, no duplicates)
- [ ] Both `.up.sql` and `.down.sql` exist
- [ ] Down migration reverses the up migration exactly
- [ ] `go build ./...` succeeds
- [ ] `docs/SCHEMA.md` updated
- [ ] `CLAUDE.md` migration count updated
- [ ] Tests pass
