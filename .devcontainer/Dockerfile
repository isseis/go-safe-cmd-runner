# x86_64 の Ubuntu をベースに使用
FROM --platform=linux/amd64 ubuntu:24.04

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

# Go 言語のインストール (例: 1.26)
RUN curl -sSfL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz -o /tmp/go.tar.gz \
    && echo "aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235  /tmp/go.tar.gz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz

# golangci-lint のインストール
RUN curl -sSfL https://github.com/golangci/golangci-lint/releases/download/v2.11.3/golangci-lint-2.11.3-linux-amd64.tar.gz \
        -o /tmp/golangci-lint.tar.gz \
    && echo "87bb8cddbcc825d5778b64e8a91b46c0526b247f4e2f2904dea74ec7450475d1  /tmp/golangci-lint.tar.gz" | sha256sum -c - \
    && tar -C /usr/local/bin -xzf /tmp/golangci-lint.tar.gz \
        --strip-components=1 golangci-lint-2.11.3-linux-amd64/golangci-lint \
    && rm /tmp/golangci-lint.tar.gz

# GitHub CLI (gh) のインストール
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
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
