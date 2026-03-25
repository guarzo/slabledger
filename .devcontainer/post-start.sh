#!/bin/bash
# Post-start script - runs every time the container starts

set -e

echo "🔄 Running post-start tasks..."

# Fix Claude Code host path references.
# When ~/.claude is mounted from the host, config files contain absolute paths
# using the host username (e.g. /home/tng/.claude/...) which don't resolve in
# the container where the user is "vscode". Create a symlink from the host home
# directory to the container home so these paths resolve correctly.
CONTAINER_USER="$(whoami)"
CONTAINER_HOME="$(eval echo ~)"
if [ -d "$CONTAINER_HOME/.claude" ]; then
    # Detect the host username from Claude's marketplace config (written by host Claude)
    HOST_HOME=""
    MARKETPLACE_CFG="$CONTAINER_HOME/.claude/plugins/known_marketplaces.json"
    if [ -f "$MARKETPLACE_CFG" ]; then
        HOST_HOME=$(grep -oP '"installLocation":\s*"\K/home/[^/]+' "$MARKETPLACE_CFG" | head -1)
    fi
    # Fallback: scan installed_plugins.json
    if [ -z "$HOST_HOME" ]; then
        INSTALLED_CFG="$CONTAINER_HOME/.claude/plugins/installed_plugins.json"
        if [ -f "$INSTALLED_CFG" ]; then
            HOST_HOME=$(grep -oP '"installPath":\s*"\K/home/[^/]+' "$INSTALLED_CFG" | head -1)
        fi
    fi
    # Create symlink if host home differs from container home
    if [ -n "$HOST_HOME" ] && [ "$HOST_HOME" != "$CONTAINER_HOME" ] && [ ! -e "$HOST_HOME" ]; then
        echo "🔗 Creating symlink $HOST_HOME -> $CONTAINER_HOME (Claude Code host path fix)"
        sudo ln -sfn "$CONTAINER_HOME" "$HOST_HOME"
    fi
fi

# Remove Windows credential helper if present (copied from host .gitconfig)
# The devcontainer already has its own credential helper in /etc/gitconfig
if grep -q "credential-manager.exe" /home/vscode/.gitconfig 2>/dev/null; then
    echo "🔧 Removing Windows credential helper from git config..."
    git config --global --unset credential.helper 2>/dev/null || true
fi

# Update Go tools to latest versions (runs in background)
(
    echo "🔧 Updating Go tools..."
    go install golang.org/x/tools/gopls@latest 2>/dev/null || true
    echo "✅ Go tools updated"
) &

# Check if .env file exists and has required variables
if [ -f ".env" ]; then
    # Check for required API keys
    if ! grep -q "PRICECHARTING_TOKEN=your_pricecharting_token_here" .env && \
       ! grep -q "PSA_ACCESS_TOKEN=your_psa_token_here" .env; then
        echo "✅ API keys configured"
    else
        echo "⚠️  Warning: API keys not configured in .env file"
        echo "   Update PRICECHARTING_TOKEN and PSA_ACCESS_TOKEN"
    fi
else
    echo "⚠️  Warning: .env file not found"
    echo "   Copy .env.example to .env and add your API keys"
fi

# Display helpful information
echo ""
echo "📊 Environment Info:"
echo "  Go version: $(go version | awk '{print $3}')"
echo "  Node version: $(node --version 2>/dev/null || echo 'not installed')"
echo "  Claude Code: $(claude --version 2>/dev/null || echo 'not installed')"
echo "  Shell: $(basename "${SHELL}")"
echo "  Working directory: $(pwd)"
echo ""

# Check Claude API key
if [ -n "$ANTHROPIC_API_KEY" ]; then
    echo "✅ ANTHROPIC_API_KEY configured"
else
    echo "⚠️  Warning: ANTHROPIC_API_KEY not set"
    echo "   Set it in .env for Claude Code CLI"
fi

# Check database
if [ -f "data/slabledger.db" ]; then
    DB_SIZE=$(du -h data/slabledger.db | awk '{print $1}')
    echo "💾 Database: data/slabledger.db ($DB_SIZE)"
else
    echo "💾 Database: not yet created (will be created on first run)"
fi

echo ""
echo "🎯 Ready to code!"
echo ""
echo "Useful commands:"
echo "  make test          # Run tests"
echo "  make build         # Build application"
echo "  make run           # Run server"
echo "  make lint          # Lint code"
echo "  claude             # Start Claude Code CLI"
echo ""
