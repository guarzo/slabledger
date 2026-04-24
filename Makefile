# SlabLedger - Makefile

.PHONY: all help build test test-verbose coverage lint check fmt clean install web web-build web-dev web-clean web-rebuild db-pull ci hooks screenshots kill

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
	@echo "  lint          Run linting and formatting"
	@echo "  check         Run full quality check (lint + architecture + file size)"
	@echo "  clean         Clean build artifacts"
	@echo "  install       Install dependencies"
	@echo ""
	@echo "Database sync (requires ~/.ssh mounted):"
	@echo "  db-pull       Pull prod DB to local"
	@echo "  # db-push     (disabled — use manually if needed)"
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
test:
	@echo "Running tests..."
	go test -race ./...

test-verbose:
	@echo "Running tests (verbose)..."
	go test -race -v ./...

coverage:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Screenshots of all pages via Playwright (uses real backend + local Postgres).
# Override SCREENSHOT_DB_URL to point at a non-devcontainer Postgres.
# Output: web/screenshots/*.png (desktop) + web/screenshots/mobile/*.png (mobile)
SCREENSHOT_TOKEN ?= playwright-screenshots
SCREENSHOT_DB_URL ?= postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable
screenshots: build web-build
	@echo "Taking screenshots of all pages (real backend)..."
	@LOCAL_API_TOKEN=$(SCREENSHOT_TOKEN) DATABASE_URL=$(SCREENSHOT_DB_URL) ./slabledger --web --port 4173 & SERVER_PID=$$! ; \
	  sleep 3 ; \
	  cd web && CI=1 SCREENSHOT_BACKEND=1 SCREENSHOT_TOKEN=$(SCREENSHOT_TOKEN) ./node_modules/.bin/playwright test tests/screenshot-all-pages.spec.ts --project=chromium ; \
	  EXIT=$$? ; kill $$SERVER_PID 2>/dev/null ; exit $$EXIT
	@echo "Screenshots saved to web/screenshots/"

# Code quality
lint:
	@echo "Formatting and linting..."
	go fmt ./...
	go vet ./...
	golangci-lint run

# Full quality check (lint + architecture + file size)
check: lint
	./scripts/check-imports.sh
	./scripts/check-file-size.sh

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

# db-push — commented out (overwrites prod DB, too dangerous for casual use)
#db-push:
#	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && \
#	PROD_DB_URL="$${PROD_DB_URL:-$$SUPABASE_URL}" && \
#	if [ -z "$$PROD_DB_URL" ]; then echo "Error: PROD_DB_URL (or SUPABASE_URL) not set"; exit 1; fi && \
#	echo "Pushing local → Supabase ..." && \
#	printf 'This will OVERWRITE THE PROD DATABASE. Type "yes" to continue: ' && read confirm && [ "$$confirm" = "yes" ] || { printf 'Aborted.\n'; exit 1; } && \
#	TIMESTAMP=$$(date +%Y%m%d%H%M%S) && \
#	TMP_REMOTE=$$(mktemp -t slabledger-remote.XXXXXX.dump) && \
#	TMP_LOCAL=$$(mktemp -t slabledger-local.XXXXXX.dump) && \
#	cleanup() { rm -f "$$TMP_REMOTE" "$$TMP_LOCAL"; } && \
#	trap cleanup EXIT && \
#	echo "Backing up remote to local file: slabledger-remote-$$TIMESTAMP.dump ..." && \
#	pg_dump --no-owner --no-privileges --format=custom --file="slabledger-remote-$$TIMESTAMP.dump" "$$PROD_DB_URL" && \
#	echo "Dumping local ..." && \
#	pg_dump --no-owner --no-privileges --format=custom --file="$$TMP_LOCAL" "$(LOCAL_DB_URL)" && \
#	echo "Resetting remote schema ..." && \
#	psql "$$PROD_DB_URL" -v ON_ERROR_STOP=1 -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;' >/dev/null && \
#	echo "Restoring local dump into remote ..." && \
#	pg_restore --no-owner --no-privileges --dbname="$$PROD_DB_URL" "$$TMP_LOCAL" && \
#	trap - EXIT && \
#	echo "Done. Remote backup: slabledger-remote-$$TIMESTAMP.dump"

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
