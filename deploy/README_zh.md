# Docker 部署教程

本文使用 Docker Hub 上的官方镜像 `lujuya/frp-web-panel` 部署，不需要克隆源码或本地构建。Master、Server、Client 使用同一个镜像，通过启动参数区分角色。

部署分两步：

1. 在 Master 主机用一份 Compose 文件部署 Master。
2. 在 Master 网页创建节点，把网页生成的 Compose 文件保存到各节点主机启动。

如果需要固定版本以便回滚，把 Compose 中的 `latest` 换成具体 tag，例如 `lujuya/frp-web-panel:local-e5fb9832191e`。

## 部署前准备

所有主机安装 Linux Docker Engine 和 Docker Compose v2（`docker compose` 命令）。公开仓库无需登录即可拉取镜像。

浏览器需要能访问 Master Web/API；Server 和 Client 需要能访问 Master API 与 RPC。默认端口为 `9000/tcp`（Web/API）和 `9001/tcp`（RPC），按实际网络路径在主机防火墙、云安全组和路由器中放行。

Server/Client 使用 Linux host 网络模式，直接占用宿主机端口。Server 上的 Caddy 还需要独占 `80/tcp` 和 `443/tcp`。

## 1. 部署 Master

### 创建目录并生成密钥

```bash
sudo mkdir -p /opt/frp-panel
cd /opt/frp-panel
openssl rand -hex 32
```

最后一条命令输出的十六进制字符串就是 `APP_GLOBAL_SECRET`，用于加密节点凭据。请保存好：重装 Master 时必须沿用同一个值，否则已有节点需要重新创建。

### 写入 .env

把上一步的输出替换到 `APP_GLOBAL_SECRET`，`MASTER_API_HOST` 换成浏览器和所有节点都能访问的 IP 或域名：

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

变量说明：

| 变量 | 说明 |
|---|---|
| `FRP_PANEL_IMAGE` | 镜像地址；回滚时改成固定 tag |
| `MASTER_WEB_PORT` / `MASTER_RPC_PORT` | Master 在宿主机发布的端口 |
| `MASTER_API_HOST` | 浏览器访问 Web/API 用的地址 |
| `MASTER_RPC_HOST` | 节点连接 RPC 用的地址 |
| `MASTER_API_SCHEME` | `http` 或 `https`，取决于是否走反向代理 |
| `CLIENT_API_URL` / `CLIENT_RPC_URL` | 写入节点 Compose 的连接地址，可填反向代理后的 HTTPS/WSS |

### 写入 docker-compose.yml

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

### 启动并注册管理员

```bash
cd /opt/frp-panel
sudo docker compose config --quiet
sudo docker compose pull
sudo docker compose up -d
sudo docker compose ps
sudo docker compose logs --tail=100 master
```

健康检查通过后，打开 `http://<MASTER_API_HOST>:9000`。**第一个注册的用户自动成为管理员**，之后注册入口关闭（`APP_ENABLE_REGISTER: "false"`）。

如果当前账户没有 Docker 权限，在 Docker 命令前加 `sudo`。

## 2. 部署 Server 和 Client 节点

节点不需要源码，也不需要本地构建，只要能拉取镜像并访问 Master。

1. 登录 Master，在「服务端」或「客户端」页面创建节点。
2. 打开节点详情页，复制或下载生成的 `docker-compose.yml`。
3. 在该节点主机创建部署目录，保存 Compose 文件：

```bash
NODE_DIR="$HOME/frp-server"   # Client 节点改为 "$HOME/frp-client"
mkdir -p "$NODE_DIR"
cd "$NODE_DIR"
umask 077
nano docker-compose.yml
chmod 600 docker-compose.yml
docker compose config --quiet
```

4. 拉取镜像并启动：

```bash
cd "$NODE_DIR"
docker compose pull
docker compose up -d
docker compose ps
docker compose logs --tail=100 -f
```

生成的 Compose 已包含节点 ID、连接密钥和 Master 连接地址，镜像固定为 `lujuya/frp-web-panel:latest` 并带 `pull_policy: always`，因此每次 `docker compose pull` 或 `up -d` 都会获取最新发布镜像。

### Server 节点额外说明

Server Compose 还会启动官方 `caddy:2-alpine`，并在 Compose 所在目录持久化 `./caddy`、`./caddy/data` 和 `./caddy/config`。**请保留整个目录**，删除 Caddy 数据会丢失已签发证书和续期状态。

Caddy 由面板生成的配置自动监听 80（HTTP-01 challenge 与 HTTP 跳转）和 443，不需要在 Caddyfile 或 Compose 里手动添加端口映射；但宿主机 80/443 不能被 `frps` 的 `vhostHTTPPort` 或其他进程占用，并需要在防火墙/安全组中放行。

证书申请失败时，在证书中心打开「查看请求记录」查看错误，并可跳转到目标服务端日志；Caddy 运行日志会写入 Caddy 数据目录的 `logs/caddy.log`。生成的 Server Compose 会把该目录只读挂载到 frp-panel 容器的 `/caddy-data`（Caddy 容器内仍是 `/data`），避免与 frp-panel 自身的 `/data`（SQLite 数据库）冲突。

### 排查节点未上线

回 Master 确认节点在线、版本和心跳正常。若节点未上线，依次检查：

- Compose 日志：`docker compose logs --tail=100 -f`
- 节点能否访问 `MASTER_API_HOST:9000` 和 `MASTER_RPC_HOST:9001`
- 防火墙或安全组是否放行 9000、9001
- 节点本地 `latest` 镜像是否过期：`docker compose pull`

## 升级

Master 和节点都通过拉取新镜像升级：

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

回滚到固定版本时，把 Compose 中的 `image` 改成具体 tag：

```yaml
image: lujuya/frp-web-panel:local-e5fb9832191e
```

## 反向代理和域名

Master 默认用 IP 直连。使用域名或反向代理时：

1. 在 `.env` 中把 `MASTER_API_HOST` 和 `MASTER_RPC_HOST` 改成域名。
2. 把 `CLIENT_API_URL` 和 `CLIENT_RPC_URL` 改成节点真正访问的地址，例如 `https://panel.example.com` 和 `wss://panel.example.com`。
3. 把 `MASTER_API_SCHEME` 改成 `https`。
4. 执行 `sudo docker compose up -d` 重建容器。

DNS 解析和反向代理（Nginx、Caddy 等）需自行配置。Web/API 走普通 HTTP 反向代理即可；RPC 需要支持 WebSocket 升级。

## 从源码构建（可选）

需要修改代码或自行构建镜像时才需要源码：

```bash
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
./scripts/build-image.sh
```

脚本按当前源码 commit 生成 `local-<commit>` tag，本地加载时同时打上 `latest`，与 Master 生成的节点 Compose 一致。

发布到 Docker Hub 需要先 `docker login`，再用 `--push`；配合 `--latest` 可同时更新 `latest`：

```bash
./scripts/build-image.sh --tag local-<commit> --platform linux/amd64 --push --latest
```

`scripts/deploy-master.sh` 保留为源码部署的便捷封装：它会构建并加载镜像、生成 `.env` 和 Compose、等待健康检查。它要求在一个干净的 Git 源码目录中运行。常规 Docker 部署按上文手动写 Compose 即可，不需要这个脚本。

完整选项可分别通过 `scripts/deploy-master.sh --help` 与 `scripts/build-image.sh --help` 查看。
