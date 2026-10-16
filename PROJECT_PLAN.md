# FRP Web 管理平台规划

## 1. 项目目标

基于 `VaalaCat/frp-panel` 构建一个以 Docker 为主的内网穿透管理平台，提供节点部署、隧道管理、HTTPS 证书管理、流量统计和日常运维能力。

目标用户不需要手动编写完整的 frpc/frps 配置，也不需要登录公网服务器修改反向代理配置。管理员在 Web 界面完成配置，目标节点通过生成的 Docker Compose 启动并连接到 Master。

基线项目：`VaalaCat/frp-panel`，本次评估的上游快照为 `1a58b85`（2026-04-19），许可证为 AGPL-3.0。阶段 0 需要重新检查上游更新后再固定依赖版本。

### 已确认的首期决策

- 用户手动在 Cloudflare 配置域名 A/AAAA 记录；首期不要求平台调用 Cloudflare DNS API。
- HTTPS 入口使用 Caddy。
- Master 使用 SQLite，并持久化数据库文件。
- Linux 节点的 FRP Server/Client Agent 默认使用 `network_mode: host`；Master 控制面和 Worker 执行环境不默认使用 host 网络。
- Worker 保留，首期平台范围为 Linux x86_64/ARM64。

## 2. 产品范围

### 2.1 本期必须支持

- Master、Server、Client 三类节点的创建、编辑、删除和状态监控。
- 创建节点后生成可复制、可下载的 Docker Compose 内容。
- 节点在线状态、最后心跳、延迟、版本、平台、地址和连接时间。
- 隧道创建、编辑、删除、启动状态查看和高级 FRP JSON 配置。
- TCP、UDP、HTTP、HTTPS 等常用 FRP 隧道表单；按 FRP 版本校验字段。
- 隧道今日、累计和历史进出流量统计。
- 近 1、3、7、30 天快捷查询及自选日期范围查询。
- 实时日志、远程控制台和可配置的远程 Shell 开关。
- Worker 创建、编辑、部署、停止、状态查看和公网入口配置。
- 免费 ACME 证书申请、续期、撤销、到期提醒和部署记录。
- 将证书部署到指定 Server 的 HTTPS 网关，并将域名路由到指定 HTTP 隧道。

### 2.2 明确移除或暂不支持

- 移除 WireGuard 组网相关页面、接口、数据模型和运行组件。
- 暂不实现主机 CPU、内存、磁盘等完整监控；一期的“监控”定义为节点和隧道运行状态。
- 不把 FRP 原生 HTTPS 透传误认为服务端证书终止；两种模式在界面中分别表达。

## 3. 总体架构

```text
浏览器
  |
  | HTTPS
  v
Master 控制平面
  |-- Web/API/RPC（Master Compose 默认使用 bridge 网络）
  |-- 用户、节点、隧道、证书、统计、审计数据
  |-- 证书状态、申请任务和续期状态
  |
  | 加密控制通道
  +--> Server Agent + frps + HTTPS Gateway
  |       |-- 公网 80/443
  |       |-- FRP HTTP/HTTPS/TCP/UDP 入口
  |       `-- 证书和域名路由
  |
  `--> Client Agent + frpc + 可选 workerd
          `-- 内网服务、Worker Unix Socket
```

### 3.1 Master

- 继承 frp-panel 的账户、权限、节点注册、配置分发和实时运维能力。
- 增加证书中心、域名路由、Compose 生成、统计查询和操作审计。
- SQLite 文件放在持久化卷中，并提供备份/恢复流程。
- 证书页面由 Master 统一管理域名、目标 Server、签发状态、到期时间和部署结果。
- 管理页面负责发起和跟踪证书申请；目标 Server 上的 Caddy 实际签发、续期并保存私钥，私钥不回传 Master。
- Master Compose 默认使用 bridge 网络并显式发布管理端口；不需要为查看节点端口而将控制面放进 host 网络。

### 3.2 Server Agent

- 继续使用 frp-panel Server 管理 frps。
- 通过现有认证控制通道接收域名路由配置，并在本机更新 Caddy。
- Compose 中的 Caddy 和 frps 使用 host 网络，Caddy 绑定公网 80/443，frps 使用独立的 HTTP 虚拟主机端口和 FRP 映射端口；host 网络下不生成 `ports:` 映射。
- Server Agent 负责检查 Caddy 配置、触发重载并回报证书元数据；Caddy 私钥和证书文件留在 Server 的持久化数据目录。

### 3.3 Client Agent

- 继续使用 frp-panel Client 管理 frpc。
- FRP Client Compose 保存数据目录和可轮换的节点凭证，并默认使用 host 网络访问宿主机上的本地服务。
- 保留 Worker 运行时，但 Worker 代码在独立、受限的容器网络中运行；不因 Client Agent 使用 host 网络而自动授予 Worker 相同的宿主机网络权限。

### 3.4 网络模式和端口原则

| 组件 | 默认网络 | 主要原因 | 典型监听/发布端口 |
| --- | --- | --- | --- |
| Master | Docker bridge | 保留控制面隔离，按需发布 Web/API 端口 | Web/API 端口 |
| Server Agent + frps | Linux host | 直接绑定公网 FRP 端口并访问宿主机网络 | `bindPort`、`vhostHTTPPort`、TCP/UDP `remotePort` |
| Server Caddy | Linux host | 直接占用公网 `80/443`，完成 ACME challenge 和 HTTPS | `80/tcp`、`443/tcp` |
| Client Agent + frpc | Linux host | 直接访问宿主机上的内网服务，FRP 监听语义与宿主机一致 | 通常无公网监听 |
| Worker runtime | 独立 bridge/受限网络 | 限制用户代码访问宿主机和控制面 | 由 Worker Ingress 分配 |

`network_mode: host` 只用于 Linux Docker Engine。host 网络下 Compose 不应再生成 `ports:`；容器与宿主机共享监听空间，FRP/Caddy 的 bind 失败就是宿主机端口冲突。Master 和 Worker 使用 bridge/隔离网络，可以降低控制面和用户代码的网络权限。

端口检查分三层：Agent 上报的操作系统实时监听快照、Master 中已经分配的端口预留、准备应用的新配置。预检优先使用 `ss -H -ltnup`，无法读取进程名时仍保留协议、监听地址、端口和时间。扫描结果是时点观测，不是操作系统级锁；最终以服务启动时的 bind 结果为准。

## 4. HTTPS 与证书设计

### 4.1 推荐模式：Server Gateway 终止 TLS

```text
https://app.example.com:443
        |
        v
Server Gateway: 证书匹配域名并解密 TLS
        |
        v
frps vhostHTTPPort，例如 8080
        |
        v
指定 Client 的 HTTP 隧道
        |
        v
客户端本地服务 :8880
```

用户创建 HTTP 隧道时绑定 `app.example.com`，创建证书时选择该域名，部署时选择目标 Server。平台自动生成 Gateway 路由，将 Gateway 的 443 转到 frps 的 HTTP 虚拟主机端口，再按 Host 路由至对应隧道。

这种模式满足“从管理页面申请证书并部署到 Server，客户端 8880 通过公网 443 访问”的目标。域名 DNS 必须指向该 Server，80/443 必须放行，且同一域名和端口只能有一个有效路由。Caddy 反向代理到 `frps` 的 `vhostHTTPPort` 时必须保留原始 `Host`，否则 FRP 无法按域名选择隧道。

### 4.2 FRP 原生 HTTPS 透传

`type=https` 用于本地服务自身已经提供 HTTPS 的场景。TLS 通常透传到 Client，证书应位于客户端服务或 Client 侧插件；它不使用 Master 部署到 Server 的证书作为默认证书。界面需要明确提示这一点。

### 4.3 ACME 策略

- 首期由管理页面发起证书配置，目标 Server 上的 Caddy 使用 Let's Encrypt 自动签发并续期。
- 用户手动将业务域名的 A/AAAA 记录指向目标 Server；Caddy 使用 HTTP-01 或 TLS-ALPN-01 验证域名。
- HTTP-01 要求公网 TCP 80 可达；TLS-ALPN-01 要求公网 TCP 443 可达。Caddy 占用这两个入口端口。
- Master 只保存证书域名、目标 Server、颁发者、指纹、签发/到期时间、续期状态和错误信息；证书私钥留在 Caddy 的持久化目录。
- Caddy 自动续期；Master 定期从 Server Agent 获取状态，并在续期或部署失败时显示错误和重试入口。
- 首期不支持通配符证书。通配符需要 DNS-01；即使 A 记录由用户手动维护，DNS-01 仍需要 TXT 验证记录自动化或每次手动修改。后续可增加 Cloudflare API Token 集成。
- 若后续要求 Master 集中保存私钥并向 Server 下发，需要增加 DNS-01 或集中 HTTP-01 challenge 转发流程，并单独评审私钥加密、访问控制和备份风险。

## 5. 核心数据模型

- `nodes`：节点类型、名称、唯一 ID、系统架构、版本、连接地址、凭证摘要、最后心跳、状态。
- `tunnels`：客户端、服务端、名称、FRP 类型、结构化配置、原始 JSON、状态和绑定域名。
- `certificates`：域名/SAN、签发方式、颁发者、指纹、签发/到期时间、Caddy 存储标识和状态；首期不保存私钥。
- `certificate_jobs`：申请/续期任务、目标 Server、状态、开始/结束时间、错误信息和重试次数。
- `gateway_routes`：域名、监听端口、目标 Server、FRP HTTP 入口、隧道 ID、证书 ID和状态。
- `traffic_daily`：隧道、自然日、入站、出站、采集时间和数据来源。
- `operations_audit`：用户、动作、目标资源、结果、来源地址和时间。
- `workers`：Worker 代码、入口文件、配置模板、部署客户端、隔离策略、运行状态和版本。
- `node_port_snapshots`：节点最近一次发现的 TCP/UDP 监听地址、端口、协议和采集时间；进程信息按权限尽力提供。
- `port_reservations`：平台配置或待应用配置占用的节点端口、协议、资源及状态，用于检查平台内重复分配；按物理节点、协议和端口唯一。

流量按平台配置时区切分自然日。查询日期范围使用半开区间 `[start, end)`，例如 10 月 7 日全天为 `10-07 00:00` 到 `10-08 00:00`。

## 6. 页面和主要流程

### 6.1 总览

- 节点总数、在线/离线、异常隧道、证书即将到期、今日流量。
- Server、Client、Worker 和隧道的状态摘要。

### 6.2 节点管理

- 创建节点：类型、名称、部署平台、架构、版本、外部地址和能力开关。
- 节点详情：状态、心跳、延迟、版本、日志、控制台、隧道和 Compose。
- Compose 页面：生成、复制、下载、重新生成和凭证轮换；Linux Server/Client Agent 使用 `network_mode: host`，不同时生成无效的 `ports:` 映射；Master 默认使用 bridge，Worker 执行环境独立隔离；默认固定镜像版本，不使用不可追踪的 `latest`。
- 端口使用页面区分“当前监听”“平台已占用/预留”和“待应用配置将占用”，显示节点、TCP/UDP、监听地址、端口、采集时间及可确认的冲突来源；进程名作为尽力提供的信息，不作为扫描成功的前提。
- 新节点首次部署前提供可复制的 `ss` 端口预检命令；节点上线后由 Agent 定期上报端口快照。应用配置前再次扫描并检查平台预留，启动后再以实际 bind/健康状态确认结果。
- 检查结果只是扫描时点的观测，不能锁定操作系统端口；端口检查通过后仍可能被其他进程抢占，启动失败时应回报具体 bind 错误。

### 6.3 隧道管理

- 选择 Client、Server、类型和本地服务。
- HTTP/HTTPS 表单包含域名、子域名、认证、请求头和证书/网关模式。
- 高级模式保留原始 JSON，并在保存前显示结构化校验错误。
- 编辑隧道时显示受影响的证书和 Gateway 路由。

### 6.4 证书中心

- 选择业务域名和目标 Server，发起申请、续期或移除路由。
- 证书详情显示 SAN、签发时间、到期时间、指纹、绑定路由和目标 Caddy 状态。
- 操作由管理页面发起；Caddy 在目标 Server 签发、保存和续期，Master 显示任务进度和每个目标的结果，不提供私钥下载。

### 6.5 统计与运维

- 统计筛选：隧道、Client、Server、1/3/7/30 天、自定义日期。
- 图表和表格同时显示入站、出站、合计及每日明细。
- 实时日志、远程终端、连接/断开、清屏和 Shell 禁用状态。

### 6.6 Worker

- Worker 列表、代码编辑、配置模板、客户端部署、运行状态。
- 创建 Worker Ingress 时选择 Client、Server 和域名/子域名。
- Worker 入口复用普通 HTTP 路由和证书网关，但在数据模型中标记为 Worker 资源。

## 7. 分阶段开发

### 阶段 0：基线与验证

- 固定 frp-panel 上游版本并建立独立仓库/分支。
- 本地可启动 Master、Server、Client 和 Worker 最小链路。
- 明确 FRP 版本、Docker 镜像来源、数据库和时区策略。
- 用 Linux 测试节点验证 Server/Client Agent 的 host 网络、Master bridge 网络、Worker 隔离、Caddy/FRPS 端口组合、端口扫描精度和 Compose 启动预检。
- 用 Let's Encrypt staging 环境验证手动 A/AAAA 记录、HTTP-01/TLS-ALPN-01 和 Caddy 自动续期状态回报。
- 通过 AGPL-3.0 许可证评估发布方式。

### 阶段 1：节点与基础运维

- 节点 CRUD、状态心跳、按角色生成 host/bridge Compose、凭证轮换、宿主机端口快照和平台端口预留检查。
- Server/Client/隧道基础页面和已有日志、终端能力收敛。
- 移除 WireGuard 入口和构建依赖。

### 阶段 2：隧道与统计

- 完善常用 FRP 表单和高级 JSON 校验。
- 统一隧道状态模型。
- 新增按日流量表、快捷范围、自定义日期和时区处理。

### 阶段 3：证书与 HTTPS Gateway

- Master 申请/续期任务编排、Caddy HTTP-01/TLS-ALPN-01、状态同步和错误展示。
- Server Caddy Compose、域名路由、证书本地持久化和 Caddy 配置平滑重载。
- 完成 8880 -> 443 的端到端验收。

### 阶段 4：Worker 与产品化

- Worker 状态、代码/模板编辑、Ingress、部署失败恢复。
- 操作审计、告警、备份恢复、升级策略和文档完善。

## 8. 关键验收条件

- 新建 Client/Server 后，复制 Compose 即可启动节点并连接 Master。
- 节点离线、恢复、版本变化和心跳超时在页面上可识别。
- 在线 Linux 节点能区分显示 TCP/UDP 当前监听、平台预留和待应用端口；配置应用前能再次检查并给出冲突提示。
- 新节点 Compose 有可执行的端口预检步骤；端口检查说明它是时点快照，启动时仍以实际 bind 结果为准。
- TCP、UDP、HTTP、HTTPS 隧道可创建、编辑、删除，并支持原始 JSON。
- `Client:8880 -> Server:frps -> Gateway:443 -> https://app.example.com` 访问成功，证书链受浏览器信任。
- 手动配置 A/AAAA 记录后，目标 Server 的 Caddy 可通过 HTTP-01/TLS-ALPN-01 签发并自动续期；Master 页面显示状态、到期时间和错误。
- 流量能按 1/3/7/30 天和自定义日期查看，日界线按配置时区一致。
- 实时日志和终端可用；禁用远程 Shell 后终端连接被拒绝。
- Worker 可以部署到 Client、通过 Server 暴露并查看运行状态。
- WireGuard 页面、API/协议、数据模型、运行时及其未使用依赖已从产品代码中移除。
- Shadow Client 共享逻辑隧道时，代理删除、重建和客户端删除会按剩余活动引用释放端口预留，避免提前解锁。

## 9. 已确认参数与剩余边界

- 已确认：手动配置 Cloudflare A/AAAA 记录、Caddy、SQLite、Linux FRP Server/Client Agent 使用 host 网络、Worker 支持 Linux x86_64/ARM64。
- 网络模式建议按组件区分：Server 和 Client 的 FRP Agent 使用 host；Server 的 Caddy 使用 host 绑定 80/443；Master 使用 bridge；Worker 执行环境独立隔离。Docker host 网络仅作为 Linux Docker Engine 首期支持路径。
- 证书由管理页面发起和统一管理，实际由目标 Server 的 Caddy 申请、保存和续期；如果“在管理节点申请并保存”特指私钥也必须留在 Master，则需另行采用集中签发/安全下发方案，不能按当前 Caddy 本地签发设计直接实现。
- 首期不含通配符证书；如需要 `*.example.com`，需确认是否提供 Cloudflare API Token 供 DNS-01 自动续期。
- host 网络首期面向 Linux Docker Engine。Docker Desktop/Windows/macOS 的 host 网络语义不同，需另做 bridge 模式模板或暂不支持。
- host 网络让 FRP Agent 能直接访问宿主机本地服务并观察宿主机监听端口，但端口快照不能消除扫描后的竞争；节点启动时的 bind 成功/失败才是最终结果。
- Master 可检查平台自己的端口配置和已登记节点状态；只有在线 Agent 才能提供目标 Linux 主机当前端口快照。新节点首次部署前需由用户运行 Compose 页面提供的本机预检命令。

## 10. 远端阶段性验收

- 在 Ubuntu 24.04 Docker 测试环境中，Master、Server Agent、Client Agent 和独立 Caddy 均能启动并保持心跳；滚动升级保留了回滚容器和 SQLite 备份。
- 已验证 Client/Server Compose 参数、SQLite WAL 持久化、节点在线状态、端口快照与冲突提示、HTTP/TCP/UDP 隧道恢复、流量统计、实时日志和 Caddy Gateway 配置同步。
- 已验证远程 Shell 默认开启；临时设置 `CLIENT_FEATURES_ENABLE_REMOTE_SHELL=false` 后 Agent 拒绝终端请求，恢复容器且不设置该变量后默认能力恢复。
- 已验证 `域名:18443 -> Caddy -> frps vhostHTTPPort:18080 -> Client` 的配置和热加载，Gateway 状态为 `active`。测试服务器没有公网 IP，`.local` 测试域名无法通过公共 ACME 校验，因此证书保持 `pending`，真实签发、续期和浏览器信任链待公网节点验证。
- 已验证证书 `autoRenew=false` 更新后写入 SQLite；证书私钥不会回传 Master。
