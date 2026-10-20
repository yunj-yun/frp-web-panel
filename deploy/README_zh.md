# 本地构建部署教程

本文默认使用 Docker Hub 镜像部署：镜像发布者为 `lujuya`，仓库为 `lujuya/frp-web-panel`。Master、Server、Client 使用同一个镜像和 tag；也可以在每台主机本地构建后使用本地镜像。

脚本默认按当前源码 commit 自动生成 `local-<commit>` tag。使用 Docker Hub 时，先推送 `lujuya/frp-web-panel:<tag>`，再在节点 Compose 中使用同一个 tag。Master 部署完成后记下输出的 `Source commit` 和 `Image tag`；各节点也可以检出同一个 commit 后本地构建相同 tag。

## 部署前准备

所有主机安装 Linux Docker Engine 和 Docker Compose v2。使用 Docker Hub 私有仓库时，在需要拉取的主机执行 `docker login`；公开仓库无需登录。需要本地构建时，Master 和 Server/Client 主机还需要 Git、OpenSSL 和 Docker Buildx。

浏览器需要能访问 Master Web/API；Server 和 Client 需要能访问 Master API 与 RPC。默认端口为 `9000/tcp`（Web/API）和 `9001/tcp`（RPC），按实际网络路径在主机防火墙、云安全组和路由器中放行。Server/Client 使用 Linux host 网络模式。

## 1. 克隆源码并部署 Master

在 Master 主机执行：

```bash
git clone --branch main https://github.com/yunj-yun/frp-web-panel.git
cd frp-web-panel
sudo ./scripts/deploy-master.sh
```

脚本会按当前源码 commit 自动命名 tag，在当前主机构建并加载镜像，然后启动 Master，不会拉取或推送项目镜像。部署成功后会打印 `Source commit`、`Image tag` 和可直接复制到节点执行的源码构建命令。

脚本默认从主机默认路由探测 IPv4 地址作为 Master API/RPC 地址，不要求域名。这个 IP 必须能被浏览器和 Server/Client 主机访问；首次启动后脚本会输出 `Web UI` 地址。要使用域名时，可在首次部署命令增加：

```bash
sudo ./scripts/deploy-master.sh --domain panel.example.com
```

`--domain` 只设置主机名，DNS 和 HTTPS 反向代理需另行配置。Web/API 默认监听宿主机 `9000/tcp`，RPC 监听 `9001/tcp`。

部署脚本会创建 `/opt/frp-panel`、SQLite 数据目录和权限为 `600` 的 `.env`，生成随机 `APP_GLOBAL_SECRET`，校验 Compose 配置并等待 Master 健康检查。打开输出的 Web UI 地址，注册的第一个用户会成为管理员。检查运行状态：

```bash
cd /opt/frp-panel
sudo docker compose ps
sudo docker compose logs --tail=100 master
```

如果当前账户没有 Docker 权限，Docker 命令前加 `sudo`。升级和备份时保留 `.env` 与 `data/`，它们包含密钥和 SQLite 数据。

## 2. 在 Server/Client 主机本地构建镜像

在每台 Server/Client 主机终端，将 Master 部署输出中 `Node source checkout and build commands` 标题下的命令原样复制粘贴执行。命令会创建或更新独立源码目录、检出 Master 使用的完整 commit，并在本机加载匹配 tag 的镜像；无需手动设置版本变量或镜像参数。

构建脚本会自动将本机架构镜像加载到 Docker。节点之间不需要共享镜像文件，也不需要登录镜像仓库；x86_64 和 ARM 主机分别在本机执行构建即可。若当前账户没有 Docker 权限，请给 Docker 命令加 `sudo`。

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

在 Compose 所在目录启动：

```bash
docker compose up -d
docker compose ps
docker compose logs --tail=100 -f
```

Master 生成的应用服务使用 `lujuya/frp-web-panel:<tag>` 和 `pull_policy: always`，执行 `docker compose pull` 或 `docker compose up -d` 时会从 Docker Hub 获取应用镜像。Server Compose 中的官方 `caddy:2-alpine` 也会从 Docker Hub 拉取。部署目录和源码目录应分开：Server 的 Caddy 会在 Compose 当前目录创建并持久化 `./caddy`、`./caddy/data` 和 `./caddy/config`。启用 HTTPS Gateway 前，确认宿主机 80/443 端口可用；请保留 Caddy 数据目录，否则会丢失证书和续期状态。

回 Master 查看节点状态，确认 Server/Client 在线、版本和心跳正常。若节点未上线，检查 Compose 日志、Compose 中的 Master 地址、节点到 Master 的 `9000/9001` 连通性，以及镜像 tag 是否一致。

## 升级

在 Master 代码目录更新到新的 `main` commit 并本地重建部署：

```bash
git pull --ff-only
sudo ./scripts/deploy-master.sh
```

Master 脚本会打印新的节点源码构建命令。在每台 Server/Client 主机复制执行，再在 Master 为节点重新生成 Compose，替换对应节点目录中的文件，最后执行 `docker compose up -d`。应用服务只使用本地镜像；Server 的 Caddy 镜像会按需拉取。升级前备份 `/opt/frp-panel/.env` 和 `/opt/frp-panel/data/`；保留 Master 中的节点记录以及 Server 的 Caddy 数据目录。

## 可选参数

常规部署无需传参。需要调整时，可在命令后增加相应选项：

- `--domain <域名>`：设置 Master 的 API/RPC 主机名；默认自动探测 IP。它等价于 `--api-host <IP/域名>`。
- `--rpc-host <IP/域名>`：单独设置 RPC 主机名；默认与 API 主机名相同。
- `--client-api-url <URL>`、`--client-rpc-url <URL>`：覆盖节点访问 Master 的 API 和 RPC 地址；反向代理时可分别设置为 HTTPS 和 WSS 地址。
- `--web-port <端口>`、`--rpc-port <端口>`：修改 Web/API 和 RPC 端口，默认分别为 `9000`、`9001`。
- `--dir <目录>`：修改 Master 部署目录，默认 `/opt/frp-panel`。
- `--tag <标签>`：覆盖默认的 `local-<commit>` 镜像标签；Master 输出的节点构建命令会带上同一个标签。
- `--no-build --tag <标签>`：不在 Master 本地构建，默认从镜像仓库拉取该标签。若镜像已提前加载到本机，可再加 `--skip-pull`；常规源码部署不需要这些参数。
- `scripts/build-image.sh --platform <平台>`：构建指定架构；发布 Docker Hub 时使用 `--push`，例如 `scripts/build-image.sh --tag local-<commit> --platform linux/amd64 --push`。

完整选项和环境变量可分别通过 `scripts/deploy-master.sh --help` 与 `scripts/build-image.sh --help` 查看。
