# SlabLedger - Makefile

.PHONY: all help build test test-verbose coverage lint check fmt clean install web web-build web-dev web-clean web-rebuild db-push db-pull ci hooks

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
	@echo "  lint          Run linting and formatting"
	@echo "  check         Run full quality check (lint + architecture + file size)"
	@echo "  clean         Clean build artifacts"
	@echo "  install       Install dependencies"
	@echo ""
	@echo "Database sync (requires ~/.ssh mounted):"
	@echo "  db-pull       Pull prod DB to local"
	@echo "  db-push       Push local DB to prod"
	@echo ""
	@echo "Note: Use '/usr/bin/make web' if 'make' command conflicts with shell function"

# Build
build:
	@echo "Building CLI..."
	go build -o slabledger ./cmd/slabledger

# Web server
web: web-build
	@echo "Starting web server with .env loaded..."
	@if [ -f .env ]; then \
		bash -c 'set -a && source .env && set +a && go run ./cmd/slabledger --web --port $${PORT:-8081}'; \
	else \
		echo "Warning: .env file not found"; \
		go run ./cmd/slabledger --web --port 8081; \
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

# Database sync (requires ~/.ssh mounted into devcontainer)
PROD_HOST ?= wanderer
PROD_DB   ?= /app/wanderer/data/slabledger.db
LOCAL_DB  ?= /workspace/data/slabledger.db

db-push:
	@echo "Pushing local DB to $(PROD_HOST):$(PROD_DB)..."
	@if [ ! -f "$(LOCAL_DB)" ]; then echo "Error: local DB not found at $(LOCAL_DB)"; exit 1; fi
	@printf 'This will OVERWRITE the prod database. Continue? [y/N] ' && read confirm && [ "$$confirm" = "y" ] || { printf 'Aborted.\n'; exit 1; }
	@LOCAL_SNAPSHOT="$(LOCAL_DB).snapshot" && \
	TIMESTAMP=$$(date +%Y%m%d%H%M%S) && TMP_REMOTE="$(PROD_DB).tmp.$$$$" && \
	cleanup() { rm -f "$$LOCAL_SNAPSHOT"; ssh "$(PROD_HOST)" "rm -f '$$TMP_REMOTE'" 2>/dev/null; } && \
	trap cleanup EXIT && \
	echo "Creating consistent local snapshot (sqlite3 .backup)..." && \
	sqlite3 "$(LOCAL_DB)" ".backup '$$LOCAL_SNAPSHOT'" && \
	echo "Backing up remote DB to $(PROD_DB).bak.$$TIMESTAMP (sqlite3 backup)..." && \
	ssh "$(PROD_HOST)" "sqlite3 '$(PROD_DB)' \".backup '$(PROD_DB).bak.$$TIMESTAMP'\"" && \
	echo "Uploading snapshot to temporary path..." && \
	scp "$$LOCAL_SNAPSHOT" "$(PROD_HOST):$$TMP_REMOTE" && \
	echo "Verifying transfer..." && \
	LOCAL_SIZE=$$(wc -c < "$$LOCAL_SNAPSHOT") && \
	REMOTE_SIZE=$$(ssh "$(PROD_HOST)" "wc -c < '$$TMP_REMOTE'") && \
	if [ "$$LOCAL_SIZE" != "$$REMOTE_SIZE" ]; then \
		echo "Error: size mismatch (local=$$LOCAL_SIZE, remote=$$REMOTE_SIZE). Cleaning up."; \
		exit 1; \
	fi && \
	echo "Restoring snapshot into production DB (WAL-safe)..." && \
	ssh "$(PROD_HOST)" "sqlite3 '$(PROD_DB)' '.restore $$TMP_REMOTE' && rm -f '$$TMP_REMOTE'" && \
	rm -f "$$LOCAL_SNAPSHOT" && \
	trap - EXIT && \
	echo "Done."

db-pull:
	@echo "Pulling prod DB from $(PROD_HOST):$(PROD_DB)..."
	@printf 'This will OVERWRITE the local database. Continue? [y/N] ' && read confirm && [ "$$confirm" = "y" ] || { printf 'Aborted.\n'; exit 1; }
	@mkdir -p "$(shell dirname $(LOCAL_DB))" && \
	TMP_REMOTE="/tmp/slabledger_backup_$$$$.db" && \
	TMP_LOCAL="$(LOCAL_DB).tmp.$$$$" && \
	cleanup() { rm -f "$$TMP_LOCAL" 2>/dev/null; ssh "$(PROD_HOST)" "rm -f '$$TMP_REMOTE'" 2>/dev/null; } && \
	trap cleanup EXIT && \
	echo "Creating consistent backup on remote (sqlite3 .backup)..." && \
	ssh "$(PROD_HOST)" "sqlite3 '$(PROD_DB)' \".backup '$$TMP_REMOTE'\"" && \
	REMOTE_SIZE=$$(ssh "$(PROD_HOST)" "wc -c < '$$TMP_REMOTE'") && \
	echo "Downloading backup to temporary file..." && \
	scp "$(PROD_HOST):$$TMP_REMOTE" "$$TMP_LOCAL" && \
	ssh "$(PROD_HOST)" "rm -f '$$TMP_REMOTE'" && \
	TMP_SIZE=$$(wc -c < "$$TMP_LOCAL") && \
	if [ "$$TMP_SIZE" != "$$REMOTE_SIZE" ]; then \
		echo "Error: size mismatch (local=$$TMP_SIZE, remote=$$REMOTE_SIZE). Cleaning up."; \
		exit 1; \
	fi && \
	sqlite3 "$(LOCAL_DB)" ".restore '$$TMP_LOCAL'" && rm -f "$$TMP_LOCAL" && \
	trap - EXIT && \
	echo "Done."

# CI target
ci: install lint test coverage build
	@echo "CI complete"

# Git hooks
hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "Git hooks installed."
