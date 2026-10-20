# FRP Web Panel

[English](./README_EN.md)

FRP Web Panel 是一个面向自托管环境的 FRP 管理与监控平台。它以 `VaalaCat/frp-panel` 为上游基础，针对实际的 Docker 部署、节点运维、端口检查、流量统计和 Caddy HTTPS 网关进行了二次开发。

项目的核心目标是：管理员在 Master 网页中管理 Server、Client、Worker、隧道和证书；节点页面直接生成可复制的 Docker Compose；节点运行状态、日志、端口快照和流量历史集中回到 Master 查看。

## 功能概览

- **Master 控制面**：用户、权限、节点、隧道、证书、网关和操作状态统一管理。
- **Server 管理**：部署和管理 `frps`，生成 FRP Server Compose，并可同时运行 Caddy HTTPS 网关。
- **Client 管理**：部署和管理 `frpc`，连接内网服务，接收 Master 下发的隧道配置。
- **Worker 管理**：保留 Linux Worker 的创建、部署、运行状态和入口管理能力。
- **隧道管理**：创建、编辑、删除和启停隧道；覆盖 TCP、UDP、HTTP、HTTPS 等常用模式，并保留高级 FRP JSON 配置。
- **节点监控**：查看在线状态、心跳、延迟、版本、平台、连接时间和最近端口快照。
- **端口检查**：展示节点当前监听的 TCP/UDP 端口，并结合平台内已分配端口检查潜在冲突。
- **流量统计**：查看隧道进出流量，支持近 1/3/7/30 天和自定义日期范围，并保留历史按日数据。
- **实时运维**：查看节点实时日志、连接/断开节点和使用远程终端。远程 Shell 可通过配置关闭。
- **证书与 HTTPS**：在证书页面登记域名、选择目标 Server 和 ACME challenge；由目标 Server 上的 Caddy 签发、续期和保存证书。
- **Docker 优先**：Master、Server、Client 使用同一镜像，通过启动命令选择角色；网页生成节点 Compose，避免手写长命令和凭据。

## 架构

```text
浏览器
  |
  | Web/API
  v
Master（SQLite、节点/隧道/证书/统计管理）
  |                         |
  | 加密控制通道              | 生成 Compose / 下发配置
  v                         v
Server Agent + frps + Caddy  Client Agent + frpc + 可选 Worker
  |                         |
公网 FRP 端口、80/443         内网服务，例如 127.0.0.1:8880
```

### 网络模式

| 组件 | 默认网络 | 说明 |
| --- | --- | --- |
| Master | Docker bridge | 仅发布 Web/API 和 RPC 端口，SQLite 放在持久化目录。 |
| Server Agent + frps | Linux host | 直接绑定宿主机的 FRP 端口和 Caddy 的 80/443。 |
| Caddy | Linux host | 与 Server 共用宿主机网络，负责 ACME 和 HTTPS 入口。 |
| Client Agent + frpc | Linux host | 直接访问宿主机上的内网服务。 |
| Worker | Linux 节点上的 Worker 运行时 | 由 Client Agent 管理，首期面向 Linux x86_64/ARM64。 |

Server 和 Client 的 Compose 使用 `network_mode: host`，因此 Compose 不再生成 `ports:` 映射；宿主机端口冲突会直接表现为服务 bind 失败。Master 使用 bridge 网络，默认发布 `9000/tcp`（Web/API）和 `9001/tcp`（RPC）。

## HTTPS 网关和证书

推荐的 HTTP 隧道访问链路如下：

```text
https://app.example.com:443
        -> Server Caddy
        -> frps HTTP vhost 端口
        -> 指定 Client 的 HTTP 隧道
        -> 客户端本地服务 :8880
```

也就是说，客户端本地端口 `8880` 可以通过 HTTP 隧道映射到 FRP 的 HTTP 虚拟主机端口，再由 Server 上的 Caddy 使用域名证书监听 `443`。Caddy 会保留原始 `Host`，使 `frps` 能按域名选择正确的隧道。

证书页面支持：

- 登记一个或多个域名并选择目标 Server。
- 选择 HTTP-01 或 TLS-ALPN-01 challenge。
- 发起申请、重新申请和自动续期状态同步。
- 将域名绑定到 HTTPS Gateway 路由，并显示 Caddy 应用状态和错误信息。
- 证书私钥只保存在目标 Server 的 Caddy 数据目录，不回传到 Master 数据库。

使用证书前需要手动在 Cloudflare 或其他 DNS 服务商配置 A/AAAA 记录，使域名指向目标 Server。平台当前不调用 Cloudflare DNS API，也不负责修改 DNS。HTTP-01 要求公网 TCP 80 可达，TLS-ALPN-01 要求公网 TCP 443 可达；Caddy 需要独占这些端口。

`type=https` 的 FRP 原生 HTTPS 隧道是另一种模式：TLS 通常透传到 Client 的本地服务，证书应由本地服务负责。它与 Server Caddy 终止 TLS 的 HTTPS Gateway 不同。首期不支持通配符证书和 DNS-01 自动化。公网 ACME 签发、自动续期和真实公网访问仍需要在具备公网地址的环境中由部署者验收。

## Docker 部署

### 环境要求

- Linux Docker Engine。
- Docker Compose v2，即 `docker compose` 命令。
- Git 和 OpenSSL；部署 Master 的脚本使用 OpenSSL 生成密钥。
- 本地构建需要 Docker Buildx；只有发布多架构镜像时才需要 QEMU。
- Server/Client 节点建议使用 Linux Docker Engine，因为它们依赖 host 网络。

Master、Server、Client 使用同一个镜像，通过 `master`、`server` 或 `client` 启动参数选择角色。默认镜像地址为：

```text
lujuya/frp-web-panel
```

首次部署时，在 Master 主机复制执行以下命令即可。默认部署会从 Docker Hub 拉取 `lujuya/frp-web-panel`。部署成功后记下输出的 `Source commit`，之后按 Master 输出的节点命令在各节点构建或拉取相同版本；自定义镜像仓库需要同时修改生成的节点 Compose 中的 `image`。

```bash
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
sudo ./scripts/deploy-master.sh
```

源码构建需要联网下载 Dockerfile 使用的基础镜像、Go 模块和 npm 依赖；这些是构建依赖下载，不会上传本项目镜像。

### 本地构建镜像

Server/Client 节点使用此命令在本机构建并加载镜像：

```bash
./scripts/build-image.sh
```

脚本默认按当前源码 commit 自动生成 `local-<commit>` tag，并将当前架构镜像加载到本机 Docker。镜像使用 `lujuya/frp-web-panel:<tag>` 作为默认名称；需要发布到 Docker Hub 时，先登录对应账号，再使用 `--push` 推送。

当前流程在每台 Master/节点主机分别本地构建，不需要多架构推送或镜像归档。首次源码构建需要联网下载 Docker 基础镜像、Go 模块和 npm 依赖。

可选参数：`scripts/build-image.sh --help` 查看完整列表。`--tag <tag>` 覆盖自动 tag，`--platform <platform>` 选择目标架构，`--push` 用于可选的镜像发布；常规部署无需添加这些参数。

### 部署 Master

默认使用本机默认路由对应的 IPv4 地址，无需先准备域名。首次部署使用上方包含克隆、进入目录和启动的完整命令；已有源码目录时运行：

```bash
sudo ./scripts/deploy-master.sh
```

部署脚本会按当前源码 commit 自动命名 tag，在 Master 主机本地构建并加载镜像，再启动容器；使用 `--no-build --tag <tag>` 时会从默认 Docker Hub 仓库拉取已有镜像。部署成功后会输出 `Source commit`、`Image tag` 和可复制的节点源码构建或拉取命令。

该 IP 会用于 Web/API、RPC 和新节点的默认连接地址，需要能被浏览器及 Server/Client 节点访问。局域网部署时通常会探测到局域网 IP。有域名时可选传 `--domain`（等价于 `--api-host`），例如：

```bash
sudo ./scripts/deploy-master.sh --domain panel.example.com
```

DNS 解析和 HTTPS 反向代理需另行配置；如 API 与 RPC 使用不同主机，增加 `--rpc-host`。反向代理场景使用 `--client-api-url` 和 `--client-rpc-url` 指定节点可访问的 HTTPS/WSS 地址。

部署脚本会：

1. 创建 `/opt/frp-panel`（可用 `--dir` 修改）和 `data/` 数据目录。
2. 生成随机 `APP_GLOBAL_SECRET`，写入权限为 `600` 的 `.env`。
3. 本地构建并加载镜像，写入并校验 Master Compose，然后启动容器。
4. 等待 `/login` 健康检查通过后输出 Web UI 地址和节点连接地址。

重复运行时会保留已有密钥和未指定的设置；显式传入的域名、端口或节点 URL 会更新 `.env` 并重建容器，因此可以先按 IP 部署，再切换到域名。原先由脚本生成的直连 HTTP/gRPC 地址会随 IP、域名和端口更新；自定义的反向代理 URL 会保留，除非再次显式传入新的 URL。

首次打开 Web UI 后注册的第一个用户会成为管理员。Compose 默认发布：

- `9000/tcp`：Web/API。
- `9001/tcp`：RPC，供节点连接。

按脚本输出的 `Web UI` 地址打开页面并注册首个管理员。可用以下命令确认 Master 容器和日志：

```bash
cd /opt/frp-panel
sudo docker compose ps
sudo docker compose logs --tail=100 master
```

反向代理或 CDN 场景下，使用 `--client-api-url` 和 `--client-rpc-url` 指定节点真正访问的 URL，例如 `https://panel.example.com` 和 `wss://panel.example.com`。备份 Master 时应同时备份 `.env` 和 `data/`，其中包含 SQLite 数据库和应用状态。

### 部署 Server 和 Client

1. 登录 Master，在 Server 或 Client 页面创建节点。
2. 打开节点详情页，复制或下载生成的 `docker-compose.yml`。
3. 按下方流程在每台节点主机本地构建镜像，将 Compose 保存到节点自己的目录并启动。

生成的 Compose 已包含节点 ID、连接凭据、Master API/RPC 地址和当前镜像 tag。Server Compose 还包含 Caddy，并将 `./caddy`、`./caddy/data` 和 `./caddy/config` 持久化到当前目录。请保留整个目录，不要删除 Caddy 数据，否则会丢失已签发证书和续期状态。

下面给出在 Server/Client 主机本地构建并使用 Master Compose 启动的完整步骤。

#### 保存 Compose 并启动

在目标节点创建独立目录，将 Master 节点详情页复制或下载的内容保存为 `docker-compose.yml`。其中含有节点 ID 和密钥，请限制文件权限：

```bash
NODE_DIR="$HOME/frp-server" # Client 节点改为 "$HOME/frp-client"
mkdir -p "$NODE_DIR"
cd "$NODE_DIR"
umask 077
nano docker-compose.yml
chmod 600 docker-compose.yml
docker compose config --quiet
```

#### 在节点主机本地构建

在每台 Server/Client 主机终端，将 Master 部署输出中 `Node source checkout and build commands` 标题下的命令原样复制粘贴执行。命令会自动创建或更新独立源码目录、检出 Master 使用的完整 commit，并在本机加载匹配 tag 的镜像；无需手动设置版本变量或镜像参数。

回到节点 Compose 目录启动：

```bash
cd "$NODE_DIR"
docker compose up -d
```

生成的应用服务使用 `lujuya/frp-web-panel:<tag>` 和 `pull_policy: always`，启动或执行 `docker compose pull` 时会从 Docker Hub 获取镜像。Server Compose 中的官方 `caddy:2-alpine` 运行依赖也会从 Docker Hub 拉取。使用固定 tag 可回滚版本；如改用本地构建镜像，可在 Compose 中替换 `image` 并改为 `pull_policy: never`。

Server Compose 还会启动 Caddy，并在节点目录保存 `./caddy`、`./caddy/data` 和 `./caddy/config`；HTTPS Gateway 需要宿主机 80/443 端口可用。请保留这些数据目录。运行 `docker compose ps` 和 `docker compose logs --tail=100 -f` 检查节点，再回 Master 确认节点在线、版本和心跳正常。常见问题是 Master 地址不可达、9000/9001 被防火墙拦截，或本地镜像与 Compose tag 不匹配。

### 升级

在 Master 源码目录更新到最新代码并重建 Master：

```bash
git pull --ff-only
sudo ./scripts/deploy-master.sh
```

Master 会打印新的节点源码构建命令。复制到每台节点执行，然后在 Master 为节点重新生成 Compose、替换节点目录里的旧文件，再运行 `docker compose up -d`。应用服务会固定使用本地镜像；Server 的 Caddy 镜像仍按需拉取。升级前备份 `/opt/frp-panel/.env` 和 `/opt/frp-panel/data/`，保留 Master 中的节点记录和节点数据目录。可选部署参数见[Docker 部署教程](./deploy/README_zh.md)。

节点上线后，Master 会显示心跳、版本、延迟、端口快照和运行状态。节点凭据属于敏感信息，不应提交到代码仓库或公开粘贴。

更完整的参数说明见 [Docker 部署说明](./deploy/README_zh.md)。

## 节点、隧道和端口

### 节点

节点页面支持创建、编辑、删除和重新生成部署内容。Server、Client 和 Worker 的状态分别显示在线、离线、连接时间、版本、平台和错误信息。Worker 保留在产品范围内，首期运行目标为 Linux x86_64/ARM64。

### 隧道

隧道可以选择 Client、Server、类型、本地地址、本地端口和远程端口。TCP、UDP、HTTP、HTTPS 等常用 FRP 配置有表单；高级模式保留原始 FRP JSON，保存前进行结构化校验。编辑或删除隧道时，页面会提示受影响的 Gateway 路由和证书绑定。

对于 TCP/UDP 隧道，Server 上的 `remotePort` 需要同时满足：端口范围合法、平台内没有重复分配、最近一次节点端口快照未发现冲突。HTTP/HTTPS 隧道使用域名和 FRP HTTP 虚拟主机逻辑，不把本地 `8880` 直接当成公网 `443` 绑定端口。

### 端口检查的边界

Agent 优先执行 `ss -H -ltnup`，不可用时回退到系统 socket 查询，并上报协议、监听地址、端口、进程和采集时间。Master 同时保存：

- 节点当前监听的时点快照。
- 平台已经分配或预留的端口。
- 正在应用的新配置将占用的端口。

快照是观测结果，不是操作系统级锁。检查通过后，其他进程仍可能抢先监听；最终以服务启动时的 bind 和健康状态为准。

## 流量、日志和远程终端

- 流量按平台配置的时区切分自然日，支持近 1、3、7、30 天快捷查询。
- 自定义查询使用半开区间 `[start, end)`；例如 10 月 7 日全天选择 `10-07 00:00` 到 `10-08 00:00`。
- 统计页面同时显示入站、出站、合计和每日明细，并可按隧道、Client、Server 筛选。
- 日志页面支持节点实时日志和 FRP/Worker 日志流。
- 远程终端通过 Master 与节点的控制通道建立 PTY 会话。生产环境如不需要该能力，可在 Master 配置 `CLIENT_FEATURES_ENABLE_REMOTE_SHELL=false` 后重启服务。

## 配置和安全建议

- 生产环境必须使用随机且足够长的 `APP_GLOBAL_SECRET`；部署脚本会自动生成。
- `.env`、SQLite `data/`、节点 Compose 和 Caddy 数据目录都可能包含凭据或证书信息，应限制文件权限并纳入备份策略。
- Master 首次注册后建议关闭公开注册，默认 Compose 已设置 `APP_ENABLE_REGISTER=false`。
- 对外发布 Master 时优先使用 HTTPS 反向代理，并为 API 和 RPC 配置正确的转发协议。
- Server 上的 Caddy 需要独占 80/443；FRP `bindPort`、HTTP vhost 端口和 TCP/UDP `remotePort` 也必须避开宿主机已有监听。
- 不要把真实域名、节点 secret、JWT、`.env` 或 Caddy 私钥提交到 Git。

## 开发

推荐环境：Go 1.24、Node.js 20、pnpm 9.15.9。前端位于 `www/`，构建产物会复制到 Go 服务的静态资源目录。

```bash
# 前端开发
cd www
pnpm install --frozen-lockfile
pnpm dev

# 前端生产构建
pnpm build

# 返回仓库根目录后构建发布二进制（跳过前端构建）
cd ..
./build.sh --skip-frontend
```

推送 `v*` Git 标签可选触发 `.github/workflows/release.yml`，创建 GitHub Release 并发布多架构 GHCR 镜像；日常部署使用 Docker Hub 上的 `lujuya/frp-web-panel`。手动发布 Docker Hub 镜像时，使用 `scripts/build-image.sh --tag <tag> --platform linux/amd64 --push`，并先执行 `docker login`。QEMU 仅用于可选的多架构镜像发布。提交前可执行静态检查：

```bash
git diff --check
bash -n scripts/build-image.sh scripts/deploy-master.sh install.sh
```

## 当前边界

- WireGuard 组网页面、接口、运行时和产品文档已移除；FRP 隧道和 Worker 仍保留。
- 暂不提供主机 CPU、内存、磁盘等完整主机监控；当前监控范围是节点、隧道、端口和流量。
- 暂不调用 Cloudflare DNS API，也不自动修改 A/AAAA 记录。
- 暂不支持通配符证书和 DNS-01 自动化。
- 当前公网 ACME 签发、自动续期、浏览器信任链和真实公网 443 访问需要在具备公网地址的环境中由部署者完成验收。

## 许可证与上游归属

本项目以 AGPL-3.0 发布，许可证和版权说明见 [LICENSE](./LICENSE)。项目基于 [VaalaCat/frp-panel](https://github.com/VaalaCat/frp-panel) 二次开发，并保留上游要求的许可证和归属信息。仓库中的 `frp-panel` 二进制、服务和容器命令名是兼容既有安装与升级流程的内部标识，不代表发布地址或镜像地址仍指向上游项目。
