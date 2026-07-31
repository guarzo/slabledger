#!/bin/bash
# Post-create script - runs once after container is created

set -eo pipefail

echo "🚀 Running post-create setup..."

# Download Go dependencies
echo "📦 Downloading Go dependencies..."
go mod download

# Install web dependencies (if package.json exists)
if [ -d "web" ] && [ -f "web/package.json" ]; then
    echo "📦 Installing web dependencies..."
    (cd web && npm install)

    # Install Playwright Chromium browser (cache lives at ~/.cache/ms-playwright)
    if [ -f "web/node_modules/.bin/playwright" ]; then
        echo "🎭 Installing Playwright Chromium browser..."
        (cd web && npx playwright install chromium) || echo "⚠️  Playwright browser install failed, continuing..."

        # Chromium's shared-lib deps (libglib2.0-0, libnss3, libgbm1, etc.) aren't
        # pulled in by `playwright install` alone — without this, headless runs
        # fail with "error while loading shared libraries: libglib-2.0.so.0".
        echo "📚 Installing Playwright OS-level dependencies..."
        (cd web && sudo env "PATH=$PATH" npx playwright install-deps chromium) || echo "⚠️  Playwright system deps install failed, continuing..."
    fi
fi

# NOTE: Claude Code CLI, Codex, the my@guarzo marketplace, and ~/.claude config
# are NOT installed here. They are owned by .devcontainer/local-seed.sh (local,
# gitignored), which runs on EVERY container start. Installing them from
# postCreateCommand was a bug: it runs once per container creation, while the
# binaries live in the ephemeral writable layer that a rebuild wipes — so after
# any rebuild `claude` silently vanished.

# Create data directories if they don't exist
echo "📁 Creating data directories..."
mkdir -p data/cache/sets data/cache/snapshots

# Set up git hooks (if .git exists)
if [ -d ".git" ]; then
    echo "🪝 Setting up git hooks..."
    git config core.hooksPath .githooks
fi

# Create .env if it doesn't exist (from .env.example)
if [ ! -f ".env" ] && [ -f ".env.example" ]; then
    echo "📝 Creating .env from .env.example..."
    cp .env.example .env
    echo "⚠️  Remember to update .env with your actual API keys!"
fi

# Build the application to verify everything works
echo "🔨 Building application..."
go build -o slabledger ./cmd/slabledger

echo "✅ Post-create setup complete!"
echo ""
echo "🎉 Development environment is ready!"
echo ""
echo "Quick start:"
echo "  - Run server: go run ./cmd/slabledger server"
echo "  - Run tests:  go test ./..."
echo "  - Run with race detector: go test -race ./..."
echo "  - Lint code:  golangci-lint run"
echo ""
echo "⚠️  Don't forget to update .env with your API keys!"
