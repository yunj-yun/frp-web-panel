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

Server Compose 中的 Caddy 由面板生成的配置自动监听 80（HTTP-01 challenge 与 HTTP 跳转）和 443，无需在 Caddyfile 或 Compose 中手动添加端口映射；但宿主机 80/443 不能被 `frps` 的 `vhostHTTPPort` 或其他进程占用，并需要在防火墙/安全组中放行。

证书申请失败时，在证书中心打开“查看请求记录”可以看到失败原因，并跳转到目标服务端的日志页面查看 ACME 细节。Caddy 的运行日志写入 Caddy 数据目录下的 `logs/caddy.log`；在生成的 Server Compose 中，该目录以只读方式挂载到 frp-panel 容器的 `/caddy-data`（Caddy 容器内仍是 `/data`），因此不会与 frp-panel 自己的 `/data` 数据目录冲突。

`type=https` 的 FRP 原生 HTTPS 隧道是另一种模式：TLS 通常透传到 Client 的本地服务，证书应由本地服务负责。它与 Server Caddy 终止 TLS 的 HTTPS Gateway 不同。首期不支持通配符证书和 DNS-01 自动化。公网 ACME 签发、自动续期和真实公网访问仍需要在具备公网地址的环境中由部署者验收。

## Docker 部署

本节只使用 Docker Hub 上的官方镜像，不需要克隆源码或在本地构建。Master 用一份 Compose 文件部署，Server/Client 节点则在 Master 网页里生成各自的 Compose 文件后部署。

### 环境要求

- Linux Docker Engine。
- Docker Compose v2，即 `docker compose` 命令。
- Server/Client 节点建议使用 Linux，因为它们依赖 host 网络共享宿主机端口。

Master、Server、Client 使用同一个镜像，通过 `master`、`server` 或 `client` 启动参数选择角色：

```text
lujuya/frp-web-panel:latest
```

镜像支持 `linux/amd64`。如需固定版本以便回滚，把 Compose 中的 `latest` 换成具体 tag（例如 `lujuya/frp-web-panel:local-e5fb9832191e`）。

### 部署 Master

1. 在 Master 主机创建部署目录：

```bash
sudo mkdir -p /opt/frp-panel
cd /opt/frp-panel
```

2. 生成密钥。`APP_GLOBAL_SECRET` 用于加密节点凭据，请保存好，后续重装要沿用同一个值：

```bash
openssl rand -hex 32
```

3. 把上一步输出的值填入 `.env`（把 `REPLACE_WITH_THE_GENERATED_SECRET` 换成实际值）。`MASTER_API_HOST` 必须是浏览器和所有 Server/Client 节点都能访问的 IP 或域名：

```bash
sudo tee /opt/frp-panel/.env >/dev/null <<'EOF'
COMPOSE_PROJECT_NAME=frp-panel
FRP_PANEL_CONTAINER_NAME=frp-panel-master
FRP_PANEL_IMAGE=lujuya/frp-web-panel:latest
MASTER_WEB_PORT=9000
MASTER_RPC_PORT=9001
APP_GLOBAL_SECRET=REPLACE_WITH_THE_GENERATED_SECRET
MASTER_API_HOST=192.0.2.10
MASTER_RPC_HOST=192.0.2.10
MASTER_API_SCHEME=http
CLIENT_API_URL=http://192.0.2.10:9000
CLIENT_RPC_URL=grpc://192.0.2.10:9001
EOF
sudo chmod 600 /opt/frp-panel/.env
```

各变量含义：

| 变量 | 说明 |
|---|---|
| `FRP_PANEL_IMAGE` | 镜像地址，回滚时改成固定 tag |
| `MASTER_WEB_PORT` / `MASTER_RPC_PORT` | Master 在宿主机发布的端口 |
| `MASTER_API_HOST` | 浏览器访问 Web/API 用的地址 |
| `MASTER_RPC_HOST` | 节点连接 RPC 用的地址 |
| `CLIENT_API_URL` / `CLIENT_RPC_URL` | 写入节点 Compose 的连接地址，可填反向代理后的 HTTPS/WSS |

4. 写入 `docker-compose.yml`：

```bash
sudo tee /opt/frp-panel/docker-compose.yml >/dev/null <<'EOF'
name: ${COMPOSE_PROJECT_NAME:-frp-panel}

services:
  master:
    image: ${FRP_PANEL_IMAGE:?Set FRP_PANEL_IMAGE in .env}
    container_name: ${FRP_PANEL_CONTAINER_NAME:-frp-panel-master}
    restart: unless-stopped
    ports:
      - "${MASTER_WEB_PORT:-9000}:9000/tcp"
      - "${MASTER_RPC_PORT:-9001}:9001/tcp"
    environment:
      APP_GLOBAL_SECRET: ${APP_GLOBAL_SECRET:?Set APP_GLOBAL_SECRET in .env}
      APP_ENABLE_REGISTER: "false"
      MASTER_API_HOST: ${MASTER_API_HOST:?Set MASTER_API_HOST in .env}
      MASTER_API_PORT: "9000"
      MASTER_API_SCHEME: ${MASTER_API_SCHEME:-http}
      MASTER_RPC_HOST: ${MASTER_RPC_HOST:?Set MASTER_RPC_HOST in .env}
      MASTER_RPC_PORT: "9001"
      CLIENT_API_URL: ${CLIENT_API_URL:?Set CLIENT_API_URL in .env}
      CLIENT_RPC_URL: ${CLIENT_RPC_URL:?Set CLIENT_RPC_URL in .env}
      DB_TYPE: sqlite3
      DB_DSN: /data/data.db?_pragma=journal_mode(WAL)
      IS_DEBUG: "false"
    volumes:
      - ./data:/data
    healthcheck:
      test: ["CMD", "curl", "--fail", "--silent", "http://127.0.0.1:9000/login"]
      interval: 15s
      timeout: 5s
      start_period: 30s
      retries: 3
EOF
```

5. 校验并启动：

```bash
cd /opt/frp-panel
sudo docker compose config --quiet
sudo docker compose pull
sudo docker compose up -d
sudo docker compose ps
```

6. 等待健康检查通过后，打开 `http://<MASTER_API_HOST>:9000` 注册首个管理员。第一个注册的用户自动成为管理员，之后注册入口关闭（`APP_ENABLE_REGISTER: "false"`）。

Master 只发布两个端口：

- `9000/tcp`：Web/API，浏览器和节点都要能访问。
- `9001/tcp`：RPC，供 Server/Client 节点连接。

使用反向代理或 CDN 时，把 `CLIENT_API_URL` 和 `CLIENT_RPC_URL` 改成节点真正访问的地址，例如 `https://panel.example.com` 和 `wss://panel.example.com`，然后 `sudo docker compose up -d` 重建容器。

备份时同时保留 `.env` 和 `data/`，其中包含密钥和 SQLite 数据库。

### 部署 Server 和 Client

节点不需要源码，也不需要本地构建，只要 Docker 和网络能访问 Master。

1. 登录 Master，在「服务端」或「客户端」页面创建节点。
2. 打开节点详情页，复制或下载生成的 `docker-compose.yml`。
3. 在该节点主机创建目录并保存 Compose：

```bash
NODE_DIR="$HOME/frp-server"   # Client 节点改为 "$HOME/frp-client"
mkdir -p "$NODE_DIR"
cd "$NODE_DIR"
umask 077
nano docker-compose.yml
chmod 600 docker-compose.yml
```

4. 校验并启动：

```bash
cd "$NODE_DIR"
docker compose config --quiet
docker compose pull
docker compose up -d
docker compose ps
```

生成的 Compose 已包含节点 ID、连接密钥和 Master API/RPC 地址，应用镜像固定为 `lujuya/frp-web-panel:latest` 并带 `pull_policy: always`，因此每次 `docker compose pull` 或 `up -d` 都会拉取最新发布镜像。

Server Compose 还会启动官方 `caddy:2-alpine`，并把 `./caddy`、`./caddy/data`、`./caddy/config` 持久化到 Compose 所在目录。**请保留整个目录**，删除 Caddy 数据会丢失已签发证书和续期状态。HTTPS 网关需要宿主机 80/443 可用。

回 Master 确认节点在线、版本和心跳正常。若节点未上线，依次检查：

- Compose 日志：`docker compose logs --tail=100 -f`
- 节点能否访问 `MASTER_API_HOST:9000` 和 `MASTER_RPC_HOST:9001`
- 防火墙/安全组是否放行 9000、9001
- 节点本地 `latest` 镜像是否过期：`docker compose pull`

### 升级

Master 和节点都通过拉取新镜像升级，不需要源码：

```bash
# Master 主机
cd /opt/frp-panel
sudo docker compose pull
sudo docker compose up -d

# 每台 Server/Client 节点
cd "$NODE_DIR"
docker compose pull
docker compose up -d
```

升级前备份 Master 的 `.env` 和 `data/`，以及各节点的 Caddy 数据目录。

如果新版修改了节点 Compose 结构（例如挂载路径或启动参数），需要回 Master 为节点**重新生成 Compose**、替换节点目录中的旧文件，再执行 `docker compose up -d`。仅执行 `pull` 不会更新 Compose 文件本身。

需要固定版本回滚时，把 Compose 中的 `image` 改成具体 tag：

```yaml
image: lujuya/frp-web-panel:local-e5fb9832191e
```

### 从源码构建（可选）

需要自行修改代码或构建镜像时，才需要克隆源码：

```bash
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
./scripts/build-image.sh
```

脚本按当前源码 commit 生成 `local-<commit>` tag，本地加载时同时打上 `latest`，与 Master 生成的节点 Compose 一致。发布到 Docker Hub 需要先 `docker login`，再用 `--push`（配合 `--latest` 可同时更新 `latest`）：

```bash
./scripts/build-image.sh --tag local-<commit> --platform linux/amd64 --push --latest
```

`scripts/deploy-master.sh` 保留为源码部署的便捷封装（会构建并加载镜像、生成 `.env` 和 Compose、等待健康检查），但常规 Docker 部署按上文手动写 Compose 即可。

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
