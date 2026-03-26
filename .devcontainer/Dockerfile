FROM ubuntu:24.04

# インタラクティブな入力を無効化
ENV DEBIAN_FRONTEND=noninteractive

# システムの「最小化」を解除 (unminimize の実行)
# yes コマンドを使って、実行中の確認プロンプトにすべて 'y' で答える
RUN yes | unminimize

# 開発ツールのインストール
RUN apt-get update && apt-get install -y \
    curl \
    git \
    build-essential \
    locales \
    sudo \
    python3 \
    python3-pip \
    libssl-dev \
    pkg-config \
    man-db \
    manpages \
    manpages-dev \
    vim \
    bash-completion \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# 日本語ロケール
RUN locale-gen ja_JP.UTF-8
ENV LANG=ja_JP.UTF-8

# Go 言語と golangci-lint のインストール
# TARGETARCH は docker-compose.yml の platform 指定から Docker が自動設定する
ARG GO_VERSION=1.26.0
ARG GOLANGCI_LINT_VERSION=2.11.3
ARG GOFUMPT_VERSION=v0.6.0
ARG TARGETARCH
RUN set -eux \
    && TARGETARCH="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && case "${TARGETARCH}" in \
        amd64) \
            GO_SHA256="aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235"; \
            LINT_SHA256="87bb8cddbcc825d5778b64e8a91b46c0526b247f4e2f2904dea74ec7450475d1"; \
            ;; \
        arm64) \
            GO_SHA256="bd03b743eb6eb4193ea3c3fd3956546bf0e3ca5b7076c8226334afe6b75704cd"; \
            LINT_SHA256="ee3d95f301359e7d578e6d99c8ad5aeadbabc5a13009a30b2b0df11c8058afe9"; \
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
    && mv /root/go/bin/gofumpt /usr/local/bin/gofumpt

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
