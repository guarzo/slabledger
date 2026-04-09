#!/bin/bash
# Post-create script - runs once after container is created

set -e

echo "🚀 Running post-create setup..."

# Download Go dependencies
echo "📦 Downloading Go dependencies..."
go mod download

# Install Go tools (if not already installed)
echo "🔧 Installing Go development tools..."
go install golang.org/x/tools/gopls@latest 2>/dev/null || true
go install github.com/go-delve/delve/cmd/dlv@latest 2>/dev/null || true
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.11.3 2>/dev/null || true
go install honnef.co/go/tools/cmd/staticcheck@latest 2>/dev/null || true

# Install web dependencies (if package.json exists)
if [ -d "web" ] && [ -f "web/package.json" ]; then
    echo "📦 Installing web dependencies..."
    (cd web && npm install)
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

# Run tests to verify setup
echo "🧪 Running tests..."
go test ./... -short

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
