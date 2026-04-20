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
    fi
fi

# Ensure ~/.local/bin is on PATH for this script (and future bash sessions)
export PATH="$HOME/.local/bin:$PATH"

# Install Claude Code CLI
echo "🤖 Installing Claude Code CLI..."
if ! curl -fsSL https://claude.ai/install.sh | bash; then
    echo "⚠️  Claude Code CLI installation failed, continuing..."
fi

# Restore Claude config from backup if the main file is missing but a backup exists
CLAUDE_CFG="$HOME/.claude.json"
if [ ! -f "$CLAUDE_CFG" ]; then
    BACKUP=$(ls -t "$HOME/.claude/backups/.claude.json.backup."* 2>/dev/null | head -1)
    if [ -n "$BACKUP" ]; then
        echo "🔧 Restoring Claude config from backup..."
        cp "$BACKUP" "$CLAUDE_CFG"
    fi
fi

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

echo "🤖 Installing OpenCode CLI..."
if ! curl -fsSL https://opencode.ai/install | bash; then
    echo "⚠️  OpenCode CLI installation failed, continuing..."
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
echo "  - Claude Code: claude"
echo ""
echo "⚠️  Don't forget to update .env with your API keys!"
echo "⚠️  Set ANTHROPIC_API_KEY in .env for Claude Code CLI"
