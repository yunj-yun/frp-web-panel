# Docker 部署流程

Master、Server、Client 使用同一个镜像，通过启动命令选择运行角色，不需要分别构建三套镜像。

## 构建与发布镜像

本地构建并加载到当前 Docker：

```bash
./scripts/build-image.sh --tag v1.2.3 --load
```

需要让其他设备拉取镜像时，将同一版本推送到 GitHub Container Registry：

```bash
docker login ghcr.io -u yunj-yun
./scripts/build-image.sh --tag v1.2.3 --platform linux/amd64,linux/arm64 --push
```

`--push` 需要有 `ghcr.io/yunj-yun/frp-web-panel` 的写入权限；节点 Compose 使用相同镜像地址，并按 Master 当前版本生成 tag。首次发布后，请在 GitHub Packages 中将镜像包设为 Public，这样目标节点无需登录即可拉取。只使用 `--load` 构建的镜像仅存在于本机，也可导出后在目标设备导入：

```bash
docker save ghcr.io/yunj-yun/frp-web-panel:v1.2.3 | gzip > frp-web-panel-v1.2.3.tar.gz
# 在每台目标设备上
gzip -dc frp-web-panel-v1.2.3.tar.gz | docker load
```

## 部署 Master

默认部署不要求域名。脚本会从默认路由对应的网卡读取 IPv4 地址，作为 Master API、RPC 和新节点 Compose 的连接地址；若系统没有 `ip` 命令或路由地址，则尝试读取主机的非回环 IPv4。该地址需要能被浏览器和 Server/Client 节点访问。直接部署到局域网时会使用局域网 IP。

最简部署：

```bash
sudo ./scripts/deploy-master.sh --tag v1.2.3
```

有域名时，可用 `--domain`（或 `--api-host`）设置 Master API/RPC 使用的主机名：

```bash
sudo ./scripts/deploy-master.sh --tag v1.2.3 --domain panel.example.com
```

等价地，也可通过 `sudo env FRP_PANEL_DOMAIN=panel.example.com ./scripts/deploy-master.sh` 传入域名。DNS 解析和 TLS 反向代理需另行配置。使用独立 API/RPC 域名时，可再传 `--rpc-host`；节点连接 URL 需要反向代理/CDN 时，使用 `--client-api-url` 和 `--client-rpc-url` 指定。

部署脚本需要 Docker、Docker Compose v2 和 OpenSSL；使用 `--build` 时还需要 Docker Buildx。脚本会创建 `/opt/frp-panel`（可通过 `--dir` 修改）、SQLite 数据目录和权限为 `600` 的 `.env`，生成随机的 `APP_GLOBAL_SECRET`，校验 Compose 配置，拉取镜像、启动容器并等待健康检查。首次打开 Web UI 后注册的第一个用户为管理员；脚本不会生成或打印管理员密码。备份时请一并备份 `.env` 和 `data/`。

重复运行部署脚本会保留现有密钥和未指定的配置；显式传入的域名、端口或节点连接 URL 会更新 `.env` 并重建容器。例如首次按 IP 部署后，可切换到域名：

```bash
sudo ./scripts/deploy-master.sh --domain panel.example.com
```

脚本生成的直连 HTTP/gRPC 地址会随 IP、域名和端口更新；自定义的反向代理 URL 会保留，除非再次显式传入新的 URL。

如果在同一台机器上使用刚通过 `--load` 构建的本地镜像，可加 `--skip-pull`：

```bash
sudo ./scripts/deploy-master.sh --tag v1.2.3 --skip-pull
```

也可以由 Master 部署脚本完成本地构建和启动：

```bash
sudo ./scripts/deploy-master.sh \
  --build \
  --tag v1.2.3
```

默认节点连接地址为 `http://<自动探测的 IP 或指定域名>:9000` 和 `grpc://<RPC 主机>:9001`。开放防火墙端口时，按实际网络路径开放 Web/API 与 RPC 端口。

## 部署 Server 和 Client

Master 启动后，在 Web UI 创建 Server 或 Client。节点详情页生成的 Compose 已包含该节点的凭据、Master 连接地址和 Master 对应的镜像版本。在目标 Linux 设备上创建目录，将网页给出的内容保存为 `docker-compose.yml`，然后执行：

```bash
docker compose up -d
```

Server Compose 还包含 Caddy 与其持久化目录；请保留整个目录，并确保所需业务端口未被其他进程占用。Client/Server 节点无需独立部署脚本。节点状态、版本、心跳和端口快照可在 Master 中查看。
