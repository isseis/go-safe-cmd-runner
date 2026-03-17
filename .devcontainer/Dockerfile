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
RUN curl -OL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz \
    && rm go1.26.0.linux-amd64.tar.gz

# golangci-lint のインストール
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin v2.11.3

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

RUN mkdir -p /home/$USERNAME/.vscode-server/extensions \
    && chown -R $USERNAME:$USERNAME /home/$USERNAME

WORKDIR /home/$USERNAME
USER $USERNAME
