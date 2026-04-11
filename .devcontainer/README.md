# DevContainer Setup

This directory contains the development container configuration for SlabLedger. Using the devcontainer ensures consistent development environment across all contributors and matches the production deployment environment.

## Prerequisites

- **VS Code** with the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)
- **Docker Desktop** (or Docker Engine + Docker Compose)
- At least 4GB of RAM allocated to Docker
- At least 20GB of free disk space

## Quick Start

1. **Open in Container**:
   - Open VS Code
   - Open this repository
   - Click the green button in the bottom-left corner
   - Select "Reopen in Container"
   - Wait for the container to build (5-10 minutes first time)

2. **After Container Starts**:
   - All dependencies are automatically installed
   - Go tools are configured
   - Extensions are installed
   - Port 8081 is forwarded to your host

3. **Start Development**:
   ```bash
   # Run the application
   go run ./cmd/slabledger server

   # Run tests
   go test ./...

   # Run with race detector
   go test -race ./...

   # Lint code
   golangci-lint run

   # Start Claude Code CLI
   claude
   ```

## What's Included

### Go Development Tools
- `gopls` - Go language server
- `dlv` - Delve debugger
- `golangci-lint` - Linter
- `staticcheck` - Static analysis
- `goimports` - Import management
- `gofumpt` - Stricter gofmt

### VS Code Extensions
- Go extension with full IntelliSense
- Docker extension
- ESLint & Prettier for web development
- SQLite viewer
- GitLens
- REST Client for API testing
- Todo Tree
- Path IntelliSense

### AI Development Tools
- **Claude Code CLI** - AI coding assistant (requires `ANTHROPIC_API_KEY`)

### System Tools
- Git, Git LFS
- SQLite 3
- curl, wget
- Docker-in-Docker (for building images)
- Node.js 24 (for web development)
- vim, nano
- htop, procps
- **zsh** with **Oh My Zsh** - Default shell with plugins

## Environment Configuration

### Required: Create .env file

Copy `.env.example` to `.env` and add your API keys:

```bash
cp .env.example .env
```

Update the following in `.env`:
```bash
# Required for application
DH_ENTERPRISE_API_KEY=your_actual_key
PSA_ACCESS_TOKEN=your_actual_token

# Required for Claude Code CLI
ANTHROPIC_API_KEY=your_anthropic_api_key

# For eBay OAuth development (sandbox)
EBAY_APP_ID=your_sandbox_app_id
EBAY_CERT_ID=your_sandbox_cert_id
```

### Development Environment Variables

The devcontainer automatically sets:
- `LOG_LEVEL=debug` - Verbose logging
- `LOG_JSON=false` - Human-readable logs
- `EBAY_OAUTH_ENV=sandbox` - Use eBay sandbox

⚠️ **Important: Set your own secrets in `.env`**

The `docker-compose.yml` contains intentionally invalid placeholder values for `SESSION_SECRET` and `ENCRYPTION_KEY`. You must set real values in your local `.env` file:

```bash
# Generate secure values for local development
SESSION_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
```

⚠️ **Never use development secrets in production!**

## Container Architecture

```
┌─────────────────────────────────────────┐
│   VS Code (Host)                        │
│   - Connects to container               │
│   - Forwards ports                      │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│   Dev Container (Docker)                │
│   ┌─────────────────────────────────┐   │
│   │  Go 1.26 + Development Tools    │   │
│   └─────────────────────────────────┘   │
│   ┌─────────────────────────────────┐   │
│   │  Your Code (mounted)            │   │
│   │  /workspace                     │   │
│   └─────────────────────────────────┘   │
│   ┌─────────────────────────────────┐   │
│   │  Go Modules Cache (volume)      │   │
│   │  Persists across rebuilds       │   │
│   └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

## Shell Configuration

The devcontainer uses **zsh** with **Oh My Zsh** as the default shell.

### Oh My Zsh Setup
- **Theme**: `robbyrussell` (default)
- **Plugins**:
  - `git` - Git aliases and completions
  - `docker` - Docker completions
  - `docker-compose` - Docker Compose completions
  - `golang` - Go aliases and completions
  - `npm` - npm completions
  - `zsh-autosuggestions` - Fish-like autosuggestions
  - `zsh-syntax-highlighting` - Syntax highlighting for commands

### Project Aliases

The following aliases are preconfigured:

```bash
# Go development
gt     # go test ./...
gtr    # go test -race ./...
gtm    # go test with mocks enabled
gb     # go build ./cmd/slabledger
gr     # go run ./cmd/slabledger
grs    # go run ./cmd/slabledger server
lint   # golangci-lint run

# Web development
webi   # npm --prefix web install
webd   # npm --prefix web run dev
webt   # npm --prefix web test -- --run
webl   # npm --prefix web run lint
```

### Host Dotfiles Integration
If you have a `~/.dotfiles` directory mounted, it will be sourced automatically.

### Shell History
Command history is persisted in a Docker volume and survives container rebuilds.

## Claude Code CLI

Claude Code is installed globally and available via the `claude` command.

### Setup
1. Add your API key to `.env`:
   ```bash
   ANTHROPIC_API_KEY=your_anthropic_api_key
   ```

2. Start Claude Code:
   ```bash
   claude
   ```

### Usage
```bash
# Start interactive session
claude

# Run a single prompt
claude "explain this function"

# Show help
claude --help
```

## Volumes

The devcontainer uses volumes for persistence:

- **Workspace**: Your code is mounted from the host (live editing)
- **Go Modules**: Cached in a Docker volume (faster builds)
- **Command History**: Shell history persists across restarts (zsh and bash)

## Ports

- `8081` - Application (automatically forwarded to host)
- `2345` - Delve debugger
- `6060` - pprof profiling (if enabled)

Access the application at: http://localhost:8081

## Debugging

### Debug with VS Code

1. Set breakpoints in your code
2. Press `F5` or use "Run and Debug" panel
3. Select "Debug Go Application"

Debug configurations are in `.vscode/launch.json`.

### Debug with Delve CLI

```bash
# Debug the server
dlv debug ./cmd/slabledger -- server --port 8081

# Debug tests
dlv test ./internal/domain/auth
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with race detector (recommended before commit)
go test -race ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/domain/auth/...

# Run with verbose output
go test -v ./...

# Run integration tests (requires mocks)
GAMESTOP_MOCK=true SALES_MOCK=true POPULATION_MOCK=true go test ./...
```

## Database Management

The SQLite database is stored in `data/slabledger.db` (mounted from host).

```bash
# View database
sqlite3 data/slabledger.db

# Check tables
sqlite3 data/slabledger.db ".tables"

# View schema
sqlite3 data/slabledger.db ".schema"

# Backup database
cp data/slabledger.db data/backup_$(date +%Y%m%d).db

# Reset database (delete file, will be recreated)
rm data/slabledger.db
```

## Web Development

The web frontend is in `web/` directory:

```bash
cd web

# Install dependencies
npm install

# Run development server (with hot reload)
npm run dev

# Run tests
npm test

# Lint
npm run lint

# Type check
npm run typecheck
```

## Customization

### Add VS Code Extensions

Edit `.devcontainer/devcontainer.json`:

```json
"customizations": {
  "vscode": {
    "extensions": [
      "your.extension.id"
    ]
  }
}
```

### Install Additional Tools

Edit `.devcontainer/Dockerfile`:

```dockerfile
RUN apt-get update && apt-get install -y \
    your-package \
    && apt-get clean
```

### Add Environment Variables

Edit `.devcontainer/docker-compose.yml`:

```yaml
environment:
  YOUR_VAR: "value"
```

## Troubleshooting

### Container Won't Build

```bash
# Rebuild without cache
Cmd/Ctrl + Shift + P
> Dev Containers: Rebuild Container Without Cache
```

### Port Already in Use

```bash
# Find process using port 8081
sudo lsof -i :8081

# Kill the process (if needed)
kill -9 <PID>
```

### Slow Performance

- Increase Docker memory allocation (Docker Desktop settings)
- Ensure Go modules cache volume exists
- Check disk space: `df -h`

### Permission Issues

```bash
# Fix workspace ownership
sudo chown -R $(whoami):$(whoami) /workspace
```

### Database Locked

```bash
# Check for other processes
ps aux | grep slabledger

# Kill stale processes
pkill slabledger
```

## Performance Tips

1. **Use Go Modules Cache**: The devcontainer automatically caches modules in a volume
2. **Disable File Watching**: If using file watchers, exclude `node_modules/`, `data/`, `cache/`
3. **Use BuildKit**: Enabled by default for faster Docker builds
4. **Allocate More Memory**: Docker Desktop → Settings → Resources → Memory (8GB recommended)

## Lifecycle Scripts

### post-create.sh (runs once)
- Downloads Go dependencies
- Installs Go tools
- Installs Claude Code CLI globally
- Creates data directories
- Builds the application
- Runs initial tests

### post-start.sh (runs every time)
- Updates Go tools
- Checks .env configuration (including ANTHROPIC_API_KEY)
- Displays environment info (including Claude Code version)
- Shows helpful commands

## Differences from Production

| Feature | Development | Production |
|---------|-------------|------------|
| User | vscode (non-root) | app (non-root) |
| Shell | zsh with Oh My Zsh | sh |
| Logging | text, debug level | JSON, info level |
| OAuth | eBay sandbox | eBay production |
| Secrets | Placeholder (set in .env) | From env vars |
| Database | Local file | Docker volume |
| Debugging | Enabled (port 2345) | Disabled |
| Hot Reload | Enabled (web) | Disabled |
| Claude Code | Installed | N/A |

## Next Steps

After setting up the devcontainer:

1. ✅ Verify `.env` has your API keys
2. ✅ Run tests: `go test ./...`
3. ✅ Start server: `go run ./cmd/slabledger server`
4. ✅ Access at http://localhost:8081
5. ✅ Start implementing OAuth authentication

## Resources

- [VS Code Dev Containers Documentation](https://code.visualstudio.com/docs/devcontainers/containers)
- [Docker Documentation](https://docs.docker.com/)
- [Go Development Guide](../../docs/architecture/ARCHITECTURE.md)
- [Production Deployment Plan](../../docs/planning/PRODUCTION_DEPLOYMENT_PLAN.md)

## Support

If you encounter issues with the devcontainer:

1. Check this README for troubleshooting tips
2. Review container logs: `docker logs <container_id>`
3. Rebuild container without cache
4. Open an issue with logs and error messages
