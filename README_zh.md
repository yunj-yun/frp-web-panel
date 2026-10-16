# FRP Web Panel

FRP Web Panel 是一个根据实际部署需求从 FRP-Panel 二次开发的自托管 FRP 管理与监控面板，整合节点管理、Docker Compose 生成、端口监测、流量历史、Caddy 证书与运维功能。

## 功能范围

- 在统一的 Web 界面管理 Master、FRP Server 和 FRP Client 节点。
- 创建、编辑和删除 TCP、UDP、HTTP、HTTPS 隧道，并支持高级 FRP JSON 配置。
- 为 Client 和 Server 生成 Docker Compose 文件；Server 节点可同时部署 Caddy。
- 查看节点状态、版本、心跳、延迟和端口占用快照。
- 查看隧道实时及历史流量，并按自选日期范围查询。
- 查看节点实时日志；管理员可按需启用远程终端。
- 申请和管理证书、自动续期，并将证书部署到指定 Server。
- 保留 Linux 节点上的 Worker 管理。

## Docker 部署

Docker 是主要部署方式。Master 部署脚本会生成随机应用密钥并启动使用 SQLite 的服务。在 Web 界面创建节点后，可将生成的 Compose 文件复制到对应 Linux 设备部署。

镜像构建、Master 部署及节点部署步骤见 [Docker 部署说明](./deploy/README_zh.md)。

## 开发说明

仓库包含 Go 服务端和 Next.js Web 应用。产品范围与开发记录见 [DEVELOPMENT_PLAN.md](./DEVELOPMENT_PLAN.md) 和 [PROJECT_PLAN.md](./PROJECT_PLAN.md)。

## 上游与许可证

本项目基于 [VaalaCat/frp-panel](https://github.com/VaalaCat/frp-panel) 二次开发，并保留上游 AGPL-3.0 许可证及版权声明，详见 [LICENSE](./LICENSE)。
