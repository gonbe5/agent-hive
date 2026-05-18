# Hive 沙箱镜像 — Recommended 档
# 覆盖 agent 真实执行场景：python/node/go 三语言 + LSP + 搜索/JSON + build + git 深层
#
# 不装的：rustup / jdk / docker-cli（见 Full 档，按需再加）
# 不装 agent-browser：它在主 Hive 容器里跑，不走 sandbox.Executor

FROM ubuntu:22.04@sha256:a6d2b38300ce017add71440577d5b0a90460d0e57fd7aec21dd0d1b0761bbfb2

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    TZ=Asia/Shanghai \
    GOPATH=/home/sandbox/go \
    PATH=/home/sandbox/go/bin:/usr/local/go/bin:/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin

# ---------- 基础层：shell + 语言运行时 + 构建工具 + git 深层 ----------
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        bash ca-certificates curl wget \
        git git-lfs openssh-client \
        python3 python3-pip python3-venv \
        build-essential make patch pkg-config \
        ripgrep jq \
        unzip tar gzip xz-utils \
        tzdata \
        sudo && \
    ln -sf /usr/bin/python3 /usr/local/bin/python && \
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone && \
    git lfs install --system && \
    rm -rf /var/lib/apt/lists/*

# ---------- Node.js 22 LTS（官方 NodeSource，apt 自带太老） ----------
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    npm config set fund false && npm config set update-notifier false && \
    npm install -g \
        typescript \
        typescript-language-server \
        pyright && \
    rm -rf /var/lib/apt/lists/* /root/.npm

# ---------- Go 1.25.1（和 go.mod go 1.25.0 对齐） ----------
# TARGETARCH 由 BuildKit 自动注入（amd64 / arm64），跨架构自动适配
ARG GO_VERSION=1.25.1
ARG TARGETARCH
RUN arch="${TARGETARCH:-$(dpkg --print-architecture)}" && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" \
        -o /tmp/go.tgz && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

# ---------- gopls（Go LSP） ----------
# CGO_ENABLED=0 避开 gcc 工具链，gopls 本身是纯 Go
RUN GOPROXY=https://goproxy.cn CGO_ENABLED=0 GOBIN=/usr/local/bin /usr/local/go/bin/go install golang.org/x/tools/gopls@latest && \
    rm -rf /root/.cache/go-build /root/go/pkg

# ---------- sandbox 用户 + workdir ----------
RUN mkdir -p /home/sandbox /home/sandbox/go && \
    chmod 755 /home/sandbox && \
    chmod -R 777 /home/sandbox/go

SHELL ["/bin/bash", "-c"]

# DockerExecutor 通过 docker exec 下发命令，容器本体只要不退就行
CMD ["sleep", "infinity"]
