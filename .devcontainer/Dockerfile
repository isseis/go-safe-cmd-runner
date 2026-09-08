FROM ubuntu:26.04

# Disable interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install development tools
RUN apt-get update && apt-get install -y \
    bash-completion \
    build-essential \
    curl \
    git \
    gnupg \
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

# Claude Code and Codex CLIs are installed later, as the non-root user, via
# their native curl installers (see below) so the user can self-update them
# from inside the devcontainer without root.

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

# The workspace is a bind mount, and the mount root's owner uid is not stable:
# it reports as USER_UID from an interactive shell but as 0 from inside a git
# hook, so git rejects the repository there with "detected dubious ownership"
# and every pre-commit hook fails. Declared system-wide so it holds before any
# user-level gitconfig exists -- postCreateCommand's pre-commit install needs
# it too.
RUN git config --system --add safe.directory /workspaces/ubuntu

RUN mkdir -p /home/$USERNAME/.vscode-server/extensions \
    && mkdir -p /home/$USERNAME/.ssh \
    && mkdir -p /home/$USERNAME/.config/gh \
    && chown -R $USERNAME:$USERNAME /home/$USERNAME

WORKDIR /home/$USERNAME
USER $USERNAME

# GitHub's "web-flow" GPG key, so `git log --show-signature` can verify
# commits GitHub creates when a PR is merged via its web UI (these are signed
# with this key, not a contributor's personal key). The downloaded file
# legitimately contains more than one key (GitHub's current key plus an old
# expired one), so it's imported into a scratch keyring first and only the
# key matching the pinned fingerprint is exported into the real keyring --
# that way a compromised or substituted response can't smuggle in an extra
# key alongside (or instead of) the real one.
ARG GITHUB_WEBFLOW_GPG_FINGERPRINT=968479A1AFF927E37D1A566BB5690EEEBB952194
RUN set -eux \
    && curl -fsSL https://github.com/web-flow.gpg -o /tmp/web-flow.gpg \
    && gpg --no-default-keyring --keyring /tmp/web-flow-scratch.gpg --import /tmp/web-flow.gpg \
    && gpg --no-default-keyring --keyring /tmp/web-flow-scratch.gpg --export "${GITHUB_WEBFLOW_GPG_FINGERPRINT}" | gpg --import \
    && gpg --list-keys --with-colons "${GITHUB_WEBFLOW_GPG_FINGERPRINT}" \
    && rm -f /tmp/web-flow.gpg /tmp/web-flow-scratch.gpg

# Claude Code CLI (native installer). Installs to ~/.local/bin as this user,
# so it auto-updates in the background and `claude update` needs no root --
# unlike the apt package this replaces, which required a system-level upgrade.
RUN curl -fsSL https://claude.ai/install.sh -o install-claude.sh && bash install-claude.sh && rm install-claude.sh

# Codex CLI (native installer). Same per-user install/update model as above:
# installs to ~/.local/bin and manages its own versions under ~/.codex.
RUN curl -fsSL https://chatgpt.com/codex/install.sh -o install-codex.sh && bash install-codex.sh && rm install-codex.sh
