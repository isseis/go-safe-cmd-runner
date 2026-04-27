FROM ubuntu:24.04

# インタラクティブな入力を無効化
ENV DEBIAN_FRONTEND=noninteractive

# システムの「最小化」を解除 (unminimize の実行)
# yes コマンドを使って、実行中の確認プロンプトにすべて 'y' で答える
RUN yes | unminimize

# 開発ツールのインストール
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

# 日本語ロケール
RUN locale-gen ja_JP.UTF-8
ENV LANG=ja_JP.UTF-8

# Go 言語と golangci-lint のインストール
# TARGETARCH は docker-compose.yml の platform 指定から Docker が自動設定する
ARG GO_VERSION=1.26.2
ARG GOLANGCI_LINT_VERSION=2.11.4
ARG GOFUMPT_VERSION=v0.6.0
ARG TARGETARCH
RUN set -eux \
    && TARGETARCH="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && case "${TARGETARCH}" in \
        amd64) \
            GO_SHA256="990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282"; \
            LINT_SHA256="200c5b7503f67b59a6743ccf32133026c174e272b930ee79aa2aa6f37aca7ef1"; \
            ;; \
        arm64) \
            GO_SHA256="c958a1fe1b361391db163a485e21f5f228142d6f8b584f6bef89b26f66dc5b23"; \
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
    && rm -rf /root/go /root/.cache/go-build

# GitHub CLI (gh) のインストール
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y gh \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# pre-commit
RUN pip3 install --break-system-packages pre-commit

# パスの設定
ENV PATH=$PATH:/usr/local/go/bin

# 一般ユーザーの作成（root での開発を避けるため）
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
