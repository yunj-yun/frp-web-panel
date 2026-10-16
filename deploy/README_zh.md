# 本地构建部署教程

本文按“不推送应用镜像到 Docker Hub 或 GHCR”的方式部署：克隆代码后，在 Master 主机本地构建并启动 Master；在每台 Server/Client 主机本地构建镜像，再将 Master 生成的 Compose 保存到各节点自己的目录启动。

Master、Server、Client 使用同一个镜像和同一个版本 tag。下面按 Git commit 固定源码，并以 `local-<commit>` 作为本地镜像 tag；各节点必须使用 Master 输出的同一个 commit 和 tag。`ghcr.io/yunj-yun/frp-web-panel:<tag>` 只是 Docker 本机镜像名称，不代表需要上传到 GHCR。

## 部署前准备

所有主机安装 Linux Docker Engine 和 Docker Compose v2。Master 主机还需要 Git、OpenSSL 和 Docker Buildx；Server/Client 主机本地构建时需要 Git 和 Docker Buildx。源码构建会从网络下载 Dockerfile 中的基础镜像及 Go/npm 构建依赖，但不会将项目镜像推送到任何仓库。

浏览器需要能访问 Master Web/API；Server 和 Client 需要能访问 Master API 与 RPC。默认端口为 `9000/tcp`（Web/API）和 `9001/tcp`（RPC），按实际网络路径在主机防火墙、云安全组和路由器中放行。Server/Client 使用 Linux host 网络模式。

## 1. 克隆源码并部署 Master

在 Master 主机执行：

```bash
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
SOURCE_COMMIT="$(git rev-parse HEAD)"
VERSION="local-$(git rev-parse --short=12 HEAD)"
printf 'SOURCE_COMMIT=%s\nVERSION=%s\n' "$SOURCE_COMMIT" "$VERSION"
sudo ./scripts/deploy-master.sh --build --tag "$VERSION"
```

`--build` 会调用 `scripts/build-image.sh` 在当前主机构建并加载镜像，然后直接用该镜像启动 Master，不会拉取或推送项目镜像。不要省略 `--tag`：它固定 Master 的程序和镜像版本，Master 后续生成的节点 Compose 也会使用这个 tag。

脚本默认从主机默认路由探测 IPv4 地址作为 Master API/RPC 地址，不要求域名。这个 IP 必须能被浏览器和 Server/Client 主机访问；首次启动后脚本会输出 `Web UI` 地址。要使用域名时，可在首次部署命令增加：

```bash
sudo ./scripts/deploy-master.sh --build --tag "$VERSION" --domain panel.example.com
```

`--domain` 只设置主机名，DNS 和 HTTPS 反向代理需另行配置。Web/API 默认监听宿主机 `9000/tcp`，RPC 监听 `9001/tcp`；自定义端口可使用 `--web-port` 和 `--rpc-port`。

部署脚本会创建 `/opt/frp-panel`、SQLite 数据目录和权限为 `600` 的 `.env`，生成随机 `APP_GLOBAL_SECRET`，校验 Compose 配置并等待 Master 健康检查。打开输出的 Web UI 地址，注册的第一个用户会成为管理员。检查运行状态：

```bash
cd /opt/frp-panel
sudo docker compose ps
sudo docker compose logs --tail=100 master
```

如果当前账户没有 Docker 权限，Docker 命令前加 `sudo`。升级和备份时保留 `.env` 与 `data/`，它们包含密钥和 SQLite 数据。

## 2. 在 Server/Client 主机本地构建镜像

在每台要运行 Server 或 Client 的主机上，检出 Master 输出的相同 `SOURCE_COMMIT`，并使用相同 `VERSION` 构建本机镜像：

```bash
SOURCE_COMMIT="paste-the-commit-hash-printed-by-master"
VERSION="local-${SOURCE_COMMIT:0:12}"
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
git fetch origin main
git checkout "$SOURCE_COMMIT"
./scripts/build-image.sh --tag "$VERSION" --load
docker image inspect "ghcr.io/yunj-yun/frp-web-panel:${VERSION}" --format '{{.Id}}'
```

`--load` 将单平台镜像加载到当前主机的 Docker。节点之间不需要共享镜像文件，也不需要登录镜像仓库；x86_64 和 ARM 主机分别在本机执行构建即可。若当前账户没有 Docker 权限，请给 Docker 命令加 `sudo`。

## 3. 从 Master 取 Compose 并在节点目录启动

1. 登录 Master，在 Server 或 Client 页面创建节点。
2. 打开节点详情页，复制或下载生成的 `docker-compose.yml`。它包含节点 ID、密钥、Master 连接地址和 Master 当前版本的镜像 tag。
3. 在对应节点主机创建自己的部署目录。Server 和 Client 可使用不同目录；将复制的内容保存为 `docker-compose.yml`，文件包含节点凭据，请限制权限。

Server 示例：

```bash
NODE_DIR="$HOME/frp-server"
mkdir -p "$NODE_DIR"
cd "$NODE_DIR"
umask 077
nano docker-compose.yml
chmod 600 docker-compose.yml
docker compose config --quiet
```

Client 可把目录改成 `"$HOME/frp-client"`，并在该目录另存该 Client 对应的 Compose 文件。

确认 Compose 的 `image:` tag 与步骤 2 中构建的 `$VERSION` 一致，然后在 Compose 所在目录执行：

```bash
docker compose up -d
docker compose ps
docker compose logs --tail=100 -f
```

Master 生成的应用服务带有 `pull_policy: never`，Compose 只使用本地构建的应用镜像，不会尝试从 GHCR 拉取。Server Compose 中的官方 `caddy:2-alpine` 运行依赖在本机缺少时仍会从 Docker Hub 拉取；这不是本项目应用镜像。部署目录和源码目录应分开：Server 的 Caddy 会在 Compose 当前目录创建并持久化 `./caddy`、`./caddy/data` 和 `./caddy/config`。启用 HTTPS Gateway 前，确认宿主机 80/443 端口可用；请保留 Caddy 数据目录，否则会丢失证书和续期状态。

回 Master 查看节点状态，确认 Server/Client 在线、版本和心跳正常。若节点未上线，检查 Compose 日志、Compose 中的 Master 地址、节点到 Master 的 `9000/9001` 连通性，以及镜像 tag 是否一致。

## 升级

在 Master 代码目录更新到新的 `main` commit 并本地重建部署：

```bash
git fetch origin main
git checkout origin/main
SOURCE_COMMIT="$(git rev-parse HEAD)"
VERSION="local-$(git rev-parse --short=12 HEAD)"
printf 'SOURCE_COMMIT=%s\nVERSION=%s\n' "$SOURCE_COMMIT" "$VERSION"
sudo ./scripts/deploy-master.sh --build --tag "$VERSION"
```

记录新的 `SOURCE_COMMIT` 和 `VERSION`，每台 Server/Client 主机检出同一 commit 并本地构建同 tag 镜像。在 Master 为节点重新生成 Compose，替换对应节点目录中的文件，再执行 `docker compose up -d`。应用服务只使用本地镜像；Server 的 Caddy 镜像会按需拉取。升级前备份 `/opt/frp-panel/.env` 和 `/opt/frp-panel/data/`；保留 Master 中的节点记录以及 Server 的 Caddy 数据目录。

部署脚本的其他参数可通过 `./scripts/deploy-master.sh --help` 查看。
