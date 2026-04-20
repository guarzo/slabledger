# SlabLedger

Go 1.26 / React 19 graded-card resale ledger. Hexagonal (Clean) Architecture with PostgreSQL.

## Critical Rules

- Domain code (`internal/domain/`) must NEVER import adapter code (`internal/adapters/`). `make check` enforces this.
- All monetary values are stored as INTEGER cents internally. Convert to USD only at the API boundary.
- NEVER commit directly to `main`. Always use a feature branch.
- Keep source files under 500 lines (600 hard limit). `make check` enforces this.

## Full Reference

See `CLAUDE.md` for commands, architecture guide, testing patterns, and all development conventions.
