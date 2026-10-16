# Docker 部署流程

Master、Server、Client 使用同一个镜像，通过启动命令选择运行角色，不需要分别构建三套镜像。

## 构建与发布镜像

本地构建并加载到当前 Docker：

```bash
./scripts/build-image.sh --tag v1.2.3 --load
```

需要让其他设备拉取镜像时，将同一版本推送到镜像仓库：

```bash
docker login
./scripts/build-image.sh --tag v1.2.3 --platform linux/amd64,linux/arm64 --push
```

`--push` 需要有 `vaalacat/frp-panel` 的写入权限。节点 Compose 由 Master 网页按当前 Master 版本生成，镜像地址为 `vaalacat/frp-panel:<版本>`。因此远程节点需要能够从 Docker Hub 拉取该 tag。只使用 `--load` 构建的镜像仅存在于本机；没有该镜像仓库的写入权限时，可以将构建结果导出并在目标设备导入：

```bash
docker save vaalacat/frp-panel:v1.2.3 | gzip > frp-panel-v1.2.3.tar.gz
# 在每台目标设备上
gzip -dc frp-panel-v1.2.3.tar.gz | docker load
```

## 部署 Master

```bash
sudo ./scripts/deploy-master.sh \
  --tag v1.2.3 \
  --api-host panel.example.com \
  --rpc-host panel.example.com
```

部署脚本需要 Docker、Docker Compose v2 和 OpenSSL；使用 `--build` 时还需要 Docker Buildx。脚本会创建 `/opt/frp-panel`、SQLite 数据目录和权限为 `600` 的 `.env`，生成随机的 `APP_GLOBAL_SECRET`，拉取镜像、启动 Compose 并等待健康检查。首次打开 Web UI 后注册的第一个用户为管理员；脚本不会生成或打印管理员密码。备份时请一并备份 `.env` 和 `data/`。

如果在同一台机器上使用刚通过 `--load` 构建的本地镜像，可加 `--skip-pull`：

```bash
sudo ./scripts/deploy-master.sh \
  --tag v1.2.3 \
  --api-host 192.168.1.10 \
  --skip-pull
```

也可以由 Master 部署脚本完成本地构建和启动：

```bash
sudo ./scripts/deploy-master.sh \
  --build \
  --tag v1.2.3 \
  --api-host 192.168.1.10
```

默认节点连接地址为 `http://<api-host>:9000` 和 `grpc://<rpc-host>:9001`。使用反向代理时，通过 `--client-api-url` 和 `--client-rpc-url` 指定节点实际可访问的 HTTPS/WSS 地址，并确保代理正确转发 API 和 RPC/WebSocket 流量。开放防火墙端口时，按实际网络路径开放 Web/API 与 RPC 端口。

## 部署 Server 和 Client

Master 启动后，在 Web UI 创建 Server 或 Client。节点详情页生成的 Compose 已包含该节点的凭据、Master 连接地址和 Master 对应的镜像版本。在目标 Linux 设备上创建目录，将网页给出的内容保存为 `docker-compose.yml`，然后执行：

```bash
docker compose up -d
```

Server Compose 还包含 Caddy 与其持久化目录；请保留整个目录，并确保所需业务端口未被其他进程占用。Client/Server 节点无需独立部署脚本。节点状态、版本、心跳和端口快照可在 Master 中查看。
