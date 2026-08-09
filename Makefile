# SlabLedger - Makefile

.PHONY: all help build test test-verbose coverage test-postgres lint check fmt clean install web web-build web-dev web-clean web-rebuild db-pull db-push ci hooks screenshots screenshots-quick kill

# Default target
all: help

help:
	@echo "SlabLedger"
	@echo ""
	@echo "Essential targets:"
	@echo "  build         Build the CLI binary"
	@echo "  web           Start web server with .env loaded (production mode)"
	@echo "  web-build     Build frontend assets with Vite"
	@echo "  web-rebuild   Force clean rebuild (clears Vite cache)"
	@echo "  web-dev       Start Vite dev server with hot reload"
	@echo "  web-clean     Clean frontend build artifacts and cache"
	@echo "  test          Run all tests with mocks"
	@echo "  test-verbose  Run all tests with verbose output"
	@echo "  coverage      Run tests with coverage report"
	@echo "  screenshots   Take screenshots of all pages (Playwright, real backend)"
	@echo "  screenshots-quick  Same but skip db-pull (use existing local data)"
	@echo "  lint          Run linting and formatting"
	@echo "  check         Run full quality check (lint + architecture + file size)"
	@echo "  clean         Clean build artifacts"
	@echo "  install       Install dependencies"
	@echo ""
	@echo "Database sync (requires ~/.ssh mounted):"
	@echo "  db-pull       Pull prod DB to local"
	@echo "  db-push       (blocked — see error message)"
	@echo ""
	@echo "Local dev utilities:"
	@echo "  kill          Kill any process running on port 8081"
	@echo ""
	@echo "Note: Use '/usr/bin/make web' if 'make' command conflicts with shell function"

# Build
build:
	@echo "Building CLI..."
	go build -o slabledger ./cmd/slabledger

# Web server
LOG_FILE ?= /workspace/app.log
web: web-build
	@echo "Starting web server with .env loaded (logging to $(LOG_FILE))..."
	@> $(LOG_FILE)
	@if [ -f .env ]; then \
		bash -c 'set -a && source .env && set +a && go run ./cmd/slabledger --web --port $${PORT:-8081}' 2>&1 | tee $(LOG_FILE); \
	else \
		echo "Warning: .env file not found"; \
		go run ./cmd/slabledger --web --port 8081 2>&1 | tee $(LOG_FILE); \
	fi

# Frontend build (Vite)
web-build:
	@echo "Building frontend with Vite..."
	@cd web && npm run build

# Frontend development server
web-dev:
	@echo "Starting Vite dev server..."
	@cd web && npm run dev

# Force clean rebuild (clears cache then rebuilds)
web-rebuild: web-clean web-build
	@echo "Clean rebuild complete"

# Clean frontend artifacts (including Vite cache)
web-clean:
	@echo "Cleaning frontend build artifacts and Vite cache..."
	@rm -rf web/dist web/node_modules/.vite

# Testing
#
# -count=1 on every target below disables Go's test cache. This is a
# correctness requirement, not a style choice: the cache keys on source files,
# the build, and consulted env vars -- NOT on database contents. A DB-backed
# suite can therefore report a cached "ok" without ever connecting, which is
# exactly what happened during #537 (make test-postgres served a stale pass
# after the database had changed underneath it).
test:
	@echo "Running tests..."
	go test -race -count=1 ./...

test-verbose:
	@echo "Running tests (verbose)..."
	go test -race -count=1 -v ./...

coverage:
	@echo "Running tests with coverage..."
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Runs the Postgres adapter package against a DEDICATED throwaway database.
# That package drops schemas and truncates tables, so it must never point at
# the development database. Creates $(POSTGRES_TEST_DB) on first run.
#
# The guard below is the safety interlock: provisioning creates
# $(POSTGRES_TEST_DB), but the tests connect via POSTGRES_TEST_DSN. If someone
# overrides the DSN to point elsewhere, we would create one database and drop
# schemas in another — so refuse to run unless the DSN's database name matches.
test-postgres:
	@dsn_db=$$(printf '%s' "$(POSTGRES_TEST_DSN)" | sed -e 's/?.*$$//' -e 's#.*/##'); \
	if [ "$$dsn_db" != "$(POSTGRES_TEST_DB)" ]; then \
		echo "ERROR: POSTGRES_TEST_DSN targets database '$$dsn_db' but POSTGRES_TEST_DB is '$(POSTGRES_TEST_DB)'."; \
		echo "       These must match; override both together, e.g.:"; \
		echo "       make test-postgres POSTGRES_TEST_DB=mydb POSTGRES_TEST_DSN=postgresql://.../mydb?sslmode=disable"; \
		exit 1; \
	fi
	@echo "Ensuring $(POSTGRES_TEST_DB) database exists..."
	@psql "$(POSTGRES_ADMIN_URL)" -tc "SELECT 1 FROM pg_database WHERE datname = '$(POSTGRES_TEST_DB)'" \
		| grep -q 1 || psql "$(POSTGRES_ADMIN_URL)" -c "CREATE DATABASE \"$(POSTGRES_TEST_DB)\""
	@echo "Running Postgres package tests..."
	POSTGRES_TEST_URL="$(POSTGRES_TEST_DSN)" go test -race -count=1 ./internal/adapters/storage/postgres/...

# Screenshots of all pages via Playwright (uses real backend + local Postgres).
# Pulls prod data first via db-pull so pages render with real content.
# Override SCREENSHOT_DB_URL to point at a non-devcontainer Postgres.
# Output: web/screenshots/*.png (desktop) + web/screenshots/mobile/*.png (mobile)
SCREENSHOT_TOKEN ?= playwright-screenshots
SCREENSHOT_DB_URL ?= postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable
define run-screenshots
	@echo "Taking screenshots of all pages (real backend)..."
	@LOCAL_API_TOKEN=$(SCREENSHOT_TOKEN) DATABASE_URL=$(SCREENSHOT_DB_URL) ./slabledger --web --port 4173 & SERVER_PID=$$! ; \
	  sleep 3 ; \
	  cd web && CI=1 SCREENSHOT_BACKEND=1 SCREENSHOT_TOKEN=$(SCREENSHOT_TOKEN) ./node_modules/.bin/playwright test tests/screenshot-all-pages.spec.ts --project=chromium ; \
	  EXIT=$$? ; kill $$SERVER_PID 2>/dev/null ; exit $$EXIT
	@echo "Screenshots saved to web/screenshots/"
endef

screenshots: db-pull build web-build
	$(run-screenshots)

screenshots-quick: build web-build
	$(run-screenshots)

# Code quality
lint:
	@echo "Formatting and linting..."
	go fmt ./...
	go vet ./...
	golangci-lint run

# Full quality check (lint + architecture + file size + harvester version coupling)
check: lint
	./scripts/check-imports-test.sh
	./scripts/check-imports.sh
	./scripts/check-file-size.sh
	./scripts/check-doc-paths.sh
	./scripts/check-playwright-version.sh

fmt:
	go fmt ./...

# Maintenance
clean: web-clean
	@echo "Cleaning Go artifacts..."
	rm -f slabledger coverage.out coverage.html
	go clean

install:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy
	@echo "Installing frontend dependencies..."
	@cd web && npm install

# Database sync (Supabase ↔ local Postgres via pg_dump / pg_restore)
#
# PROD_DB_URL — Supabase connection string (session pooler, port 5432).
# LOCAL_DB_URL — devcontainer Postgres.
# Both can be overridden from the environment.
PROD_DB_URL  ?= $(SUPABASE_URL)
LOCAL_DB_URL ?= postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable
POSTGRES_ADMIN_URL ?= postgresql://slabledger:slabledger@postgres:5432/postgres?sslmode=disable
POSTGRES_TEST_DB   ?= slabledger_test
POSTGRES_TEST_DSN  ?= postgresql://slabledger:slabledger@postgres:5432/$(POSTGRES_TEST_DB)?sslmode=disable

db-pull:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && \
	PROD_DB_URL="$${PROD_DB_URL:-$$SUPABASE_URL}" && \
	if [ -z "$$PROD_DB_URL" ]; then echo "Error: PROD_DB_URL (or SUPABASE_URL) not set"; exit 1; fi && \
	echo "Pulling Supabase → local Postgres ..." && \
	if [ "$(YES)" = "1" ]; then echo "YES=1 set — skipping interactive confirmation."; else printf 'This will OVERWRITE the local database. Continue? [y/N] ' && read confirm && [ "$$confirm" = "y" ] || { printf 'Aborted.\n'; exit 1; }; fi && \
	TMP_DUMP=$$(mktemp -t slabledger-pull.XXXXXX.dump) && \
	cleanup() { rm -f "$$TMP_DUMP"; } && \
	trap cleanup EXIT && \
	echo "Dumping prod (custom format, data-only except schema_migrations) ..." && \
	pg_dump --no-owner --no-privileges --format=custom -n public --extension=citext --file="$$TMP_DUMP" "$$PROD_DB_URL" && \
	echo "Resetting local schema ..." && \
	psql "$(LOCAL_DB_URL)" -v ON_ERROR_STOP=1 -c 'DROP SCHEMA IF EXISTS public CASCADE;' >/dev/null && \
	echo "Restoring into local ..." && \
	pg_restore --no-owner --no-privileges --dbname="$(LOCAL_DB_URL)" "$$TMP_DUMP" && \
	trap - EXIT && rm -f "$$TMP_DUMP" && \
	echo "Done."

db-push:
	@echo "ERROR: db-push is intentionally disabled — it overwrites the production database." && \
	echo "Use the documented backup/restore procedure if you need to push data to prod." && \
	false

# Kill process on port 8081
kill:
	@pids=$$(lsof -ti :8081); \
	if [ -z "$$pids" ]; then \
		echo "Nothing running on :8081"; \
	elif kill $$pids; then \
		echo "Killed process on :8081"; \
	else \
		echo "Error: failed to kill process on :8081" >&2; exit 1; \
	fi

# CI target
ci: install lint test coverage build
	@echo "CI complete"

# Git hooks
hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit .githooks/post-commit
	@echo "Git hooks installed."
