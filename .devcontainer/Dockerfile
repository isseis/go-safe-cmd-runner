FROM ubuntu:26.04

# Disable interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install development tools
RUN apt-get update && apt-get install -y \
    bash-completion \
    build-essential \
    curl \
    git \
    libssl-dev \
    locales \
    man-db \
    manpages \
    manpages-dev \
    pkg-config \
    python3 \
    python3-pip \
    ripgrep \
    sudo \
    vim \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Japanese locale
RUN locale-gen ja_JP.UTF-8
ENV LANG=ja_JP.UTF-8

# Install Go and golangci-lint
# TARGETARCH is automatically set by Docker based on the platform specified in docker-compose.yml
ARG GO_VERSION=1.26.3
ARG GOLANGCI_LINT_VERSION=2.11.4
ARG GOFUMPT_VERSION=v0.6.0
ARG DEADCODE_VERSION=v0.45.0
ARG TARGETARCH
RUN set -eux \
    && TARGETARCH="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && case "${TARGETARCH}" in \
    amd64) \
    GO_SHA256="2b2cfc7148493da5e73981bffbf3353af381d5f93e789c82c79aff64962eb556"; \
    LINT_SHA256="200c5b7503f67b59a6743ccf32133026c174e272b930ee79aa2aa6f37aca7ef1"; \
    ;; \
    arm64) \
    GO_SHA256="9d89a3ea57d141c2b22d70083f2c8459ba3890f2d9e818e7e933b75614936565"; \
    LINT_SHA256="3bcfa2e6f3d32b2bf5cd75eaa876447507025e0303698633f722a05331988db4"; \
    ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac \
    && curl -sSfL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tar.gz \
    && echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz \
    && curl -sSfL "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${TARGETARCH}.tar.gz" \
    -o /tmp/golangci-lint.tar.gz \
    && echo "${LINT_SHA256}  /tmp/golangci-lint.tar.gz" | sha256sum -c - \
    && tar -C /usr/local/bin -xzf /tmp/golangci-lint.tar.gz \
    --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${TARGETARCH}/golangci-lint" \
    && rm /tmp/golangci-lint.tar.gz \
    && /usr/local/go/bin/go install mvdan.cc/gofumpt@${GOFUMPT_VERSION} \
    && mv /root/go/bin/gofumpt /usr/local/bin/gofumpt \
    && /usr/local/go/bin/go install golang.org/x/tools/cmd/deadcode@${DEADCODE_VERSION} \
    && mv /root/go/bin/deadcode /usr/local/bin/deadcode \
    && rm -rf /root/go /root/.cache/go-build

# Install GitHub CLI (gh)
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y gh \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# pre-commit
RUN pip3 install --break-system-packages pre-commit

# Claude Code CLI (apt repository, verify signing key fingerprint)
RUN apt-get update && apt-get install -y gnupg \
    && install -d -m 0755 /etc/apt/keyrings \
    && curl -fsSL https://downloads.claude.ai/keys/claude-code.asc \
    -o /etc/apt/keyrings/claude-code.asc \
    && FINGERPRINT="$(gpg --show-keys --with-colons /etc/apt/keyrings/claude-code.asc \
    | awk -F: '/^fpr:/ { print $10; exit }')" \
    && [ "${FINGERPRINT}" = "31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE" ] \
    || (echo "unexpected Claude Code signing key fingerprint: ${FINGERPRINT}" && exit 1) \
    && echo "deb [signed-by=/etc/apt/keyrings/claude-code.asc] https://downloads.claude.ai/claude-code/apt/stable stable main" \
    > /etc/apt/sources.list.d/claude-code.list \
    && apt-get update && apt-get install -y claude-code \
    && apt-get purge -y gnupg && apt-get autoremove -y \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# herdr (agent session persistence tool, verified via official manifest SHA-256)
ARG HERDR_VERSION=0.8.2
RUN set -eux \
    && TARGETARCH="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && case "${TARGETARCH}" in \
    amd64) \
    HERDR_TARGET="linux-x86_64"; \
    HERDR_SHA256="976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4"; \
    ;; \
    arm64) \
    HERDR_TARGET="linux-aarch64"; \
    HERDR_SHA256="f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d"; \
    ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac \
    && curl -fsSL "https://github.com/herdrdev/herdr/releases/download/v${HERDR_VERSION}/herdr-${HERDR_TARGET}" \
    -o /tmp/herdr \
    && echo "${HERDR_SHA256}  /tmp/herdr" | sha256sum -c - \
    && install -m 0755 /tmp/herdr /usr/local/bin/herdr \
    && rm /tmp/herdr

# Configure PATH
ENV PATH=$PATH:/usr/local/go/bin

# Create a regular user (to avoid developing as root)
ARG USERNAME=issei
ARG USER_UID=1000
ARG USER_GID=$USER_UID

RUN EXISTING_USER=$(getent passwd "$USER_UID" | cut -d: -f1) \
    && if [ -n "$EXISTING_USER" ]; then userdel -r "$EXISTING_USER"; fi \
    && groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USER_GID -m $USERNAME \
    && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

COPY .devcontainer/bashrc_git_ps1.sh /etc/bash_completion.d/git_ps1_settings.sh

RUN mkdir -p /home/$USERNAME/.vscode-server/extensions \
    && mkdir -p /home/$USERNAME/.ssh \
    && chown -R $USERNAME:$USERNAME /home/$USERNAME

WORKDIR /home/$USERNAME
USER $USERNAME
