# Development container for SlabLedger
# Based on official Go image with development tools

FROM golang:1.26

# Avoid warnings by switching to noninteractive
ENV DEBIAN_FRONTEND=noninteractive

# Use AWS-backed Debian mirror (Fastly CDN IPs for deb.debian.org are
# unreachable from some WSL2/Docker Desktop networks)
RUN sed -i 's|deb.debian.org|cdn-aws.deb.debian.org|g' /etc/apt/sources.list.d/*.sources 2>/dev/null; \
    sed -i 's|deb.debian.org|cdn-aws.deb.debian.org|g' /etc/apt/sources.list 2>/dev/null; \
    true

# Install base development tools
RUN apt-get update && apt-get install -y \
    # Build essentials
    build-essential \
    # Version control
    git \
    git-lfs \
    # Database tools
    sqlite3 \
    # Network tools
    curl \
    wget \
    netcat-openbsd \
    lsof \
    iproute2 \
    net-tools \
    rsync \
    # Process tools
    htop \
    procps \
    psmisc \
    # Filesystem tools
    tree \
    # JSON/text processing
    jq \
    ripgrep \
    # Pagers
    less \
    # Linters
    shellcheck \
    # Text editors
    vim \
    nano \
    # Shells
    bash-completion \
    zsh \
    # Compression
    zip \
    unzip \
    # TLS certificates
    ca-certificates \
    # Locales
    locales \
    # User management (needed later for vscode user)
    sudo \
    # Clean up
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Generate locale
RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
    locale-gen

ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

# Install Go tools
RUN go install golang.org/x/tools/gopls@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin v2.11.3 && \
    go install honnef.co/go/tools/cmd/staticcheck@latest && \
    go install github.com/google/addlicense@latest && \
    go install mvdan.cc/gofumpt@latest && \
    go install golang.org/x/tools/cmd/goimports@latest

# Note: Node.js and npm tools are installed via devcontainer features
# See devcontainer.json "features" section

# Create non-root user
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=$USER_UID

RUN groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USER_GID -m -s /bin/zsh $USERNAME \
    && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

# Create workspace directory
RUN mkdir -p /workspace && chown -R $USERNAME:$USERNAME /workspace

# Create command history directory
RUN mkdir -p /commandhistory && chown -R $USERNAME:$USERNAME /commandhistory

# Fix Go module cache permissions
RUN mkdir -p /go/pkg/mod /go/pkg/sumdb && chown -R $USERNAME:$USERNAME /go

# Set up shell history persistence for bash (fallback)
RUN echo "export PROMPT_COMMAND='history -a'" >> /home/$USERNAME/.bashrc \
    && echo "export HISTFILE=/commandhistory/.bash_history" >> /home/$USERNAME/.bashrc

# Set up useful aliases for bash (fallback)
RUN echo "alias ll='ls -lah'" >> /home/$USERNAME/.bashrc \
    && echo "alias gs='git status'" >> /home/$USERNAME/.bashrc \
    && echo "alias gd='git diff'" >> /home/$USERNAME/.bashrc \
    && echo "alias gt='go test ./...'" >> /home/$USERNAME/.bashrc \
    && echo "alias gtr='go test -race ./...'" >> /home/$USERNAME/.bashrc

# Ensure ~/.local/bin and ~/.opencode/bin are on PATH for bash
RUN echo 'export PATH="$HOME/.local/bin:$HOME/.opencode/bin:$PATH"' >> /home/$USERNAME/.bashrc

# Enable bash completion
RUN echo "source /etc/bash_completion" >> /home/$USERNAME/.bashrc

# Install OpenCode AI coding agent
RUN curl -fsSL https://opencode.ai/install | bash

# Install Oh My Zsh for vscode user
USER $USERNAME
RUN sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended

# Install zsh plugins
RUN git clone https://github.com/zsh-users/zsh-autosuggestions ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-autosuggestions \
    && git clone https://github.com/zsh-users/zsh-syntax-highlighting ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-syntax-highlighting

# Switch back to root for config file setup
USER root

# Create zsh config with Oh My Zsh
RUN printf '%s\n' \
    '# SlabLedger devcontainer zsh config with Oh My Zsh' \
    'export ZSH="$HOME/.oh-my-zsh"' \
    '' \
    '# Theme - robbyrussell is default, agnoster is popular for powerline fonts' \
    'ZSH_THEME="robbyrussell"' \
    '' \
    '# Plugins' \
    'plugins=(' \
    '  git' \
    '  docker' \
    '  docker-compose' \
    '  golang' \
    '  npm' \
    '  zsh-autosuggestions' \
    '  zsh-syntax-highlighting' \
    ')' \
    '' \
    'source $ZSH/oh-my-zsh.sh' \
    '' \
    '# History settings (override Oh My Zsh defaults for persistence)' \
    'HISTFILE=/commandhistory/.zsh_history' \
    'HISTSIZE=10000' \
    'SAVEHIST=10000' \
    '' \
    '# Project aliases' \
    'alias ll="ls -lah"' \
    'alias la="ls -A"' \
    '' \
    '# Go development aliases' \
    'alias gt="go test ./..."' \
    'alias gtr="go test -race ./..."' \
    'alias gtm="GAMESTOP_MOCK=true SALES_MOCK=true POPULATION_MOCK=true go test ./..."' \
    'alias gb="go build ./cmd/slabledger"' \
    'alias gr="go run ./cmd/slabledger"' \
    'alias grs="go run ./cmd/slabledger server"' \
    'alias lint="golangci-lint run"' \
    '' \
    '# Web development aliases' \
    'alias webi="npm --prefix web install"' \
    'alias webd="npm --prefix web run dev"' \
    'alias webt="npm --prefix web test -- --run"' \
    'alias webl="npm --prefix web run lint"' \
    '' \
    '# AI coding agents' \
    'alias claude="claude"' \
    'alias oc="opencode"' \
    '' \
    '# Ensure Claude Code, OpenCode, and local binaries are on PATH' \
    'export PATH="$HOME/.local/bin:$HOME/.opencode/bin:$PATH"' \
    '' \
    'export EDITOR=vim' \
    'cd /workspace 2>/dev/null || true' \
    > /home/$USERNAME/.zshrc \
    && chown $USERNAME:$USERNAME /home/$USERNAME/.zshrc

# Configure git to be safe with workspace directory
RUN git config --system --add safe.directory /workspace

# Switch to non-root user
USER $USERNAME

# Set working directory
WORKDIR /workspace

# Switch back to dialog for any ad-hoc use of apt-get
ENV DEBIAN_FRONTEND=dialog

# Default command
CMD ["sleep", "infinity"]
