# FRP Web 详细开发计划

## 0. 开发基线

- 上游项目：`VaalaCat/frp-panel`
- 固定基线：`1a58b856d7de19de8669b7072872986d2fa1604a`
- 后端：Go 1.24、Gin、GORM、SQLite WAL、gRPC/Protobuf
- 前端：Next.js 14 Pages Router、React 18、TypeScript、Tailwind、Radix UI、React Query、Recharts、xterm
- 部署：Master、Server、Client 和 Worker 均提供 Docker Compose；Linux FRP Agent/Caddy 使用 host 网络，Master 和 Worker 使用隔离网络
- 许可证：继承 AGPL-3.0，发布前保留版权和许可证说明

当前工作顺序是先导入并验证上游，再以小的可回滚增量增加产品能力。每个增量都必须通过格式检查、单元测试、构建和必要的浏览器验收。

## 1. 产品目标和首期边界

首期产品要让管理员在一个 Web 控制台内完成以下流程：创建节点、复制对应 Compose、查看节点状态和端口、创建隧道、发布 HTTP/TCP/UDP 服务、申请和跟踪 HTTPS 证书、查看流量、读取实时日志，并按配置决定是否开放远程 Shell。

明确不做：WireGuard 组网页面和运行组件、首期 Cloudflare DNS API、主机级完整监控平台、首期通配符 DNS-01 证书。Worker 保留，首期只支持 Linux x86_64/ARM64。

## 2. 目标架构

```text
Browser
  | HTTPS
  v
Master (Go API + Next.js + SQLite)
  | encrypted control/RPC channel
  +--> Server Agent + frps + Caddy (Linux host network)
  |       |-- Caddy :80/:443, ACME and HTTPS gateway
  |       `-- frps bind/vhost/remote ports
  `--> Client Agent + frpc (Linux host network)
          `-- optional Worker runtime (isolated bridge network)
```

Master 是配置和状态的权威来源。Server/Client Agent 负责执行配置、心跳、日志、端口快照和能力回报。Caddy 在目标 Server 实际申请、续期和保存证书私钥，Master 只保存证书元数据和任务状态。

### 2.1 网络模式

| 组件 | 网络模式 | 说明 |
| --- | --- | --- |
| Master | bridge | 显式发布 Web/API/RPC 端口；管理面不需要 host 网络 |
| Server Agent + frps | Linux host | 直接绑定公网 FRP 端口，不能同时生成 `ports:` |
| Server Caddy | Linux host | 直接绑定 `80/tcp` 和 `443/tcp`，不能同时生成 `ports:` |
| Client Agent + frpc | Linux host | 直接访问宿主机上的本地服务 |
| Worker runtime | 独立 bridge/受限网络 | 默认禁止访问 Docker socket、Master 数据目录和宿主机网络 |

### 2.2 HTTPS 路径

```text
https://app.example.com:443
  -> Caddy :443, certificate for app.example.com
  -> frps vhostHTTPPort, preserve original Host header
  -> FRP HTTP tunnel
  -> Client local service :8880
```

FRP `type=https` 仍作为高级/透传模式保留，但它的 TLS 通常在 Client 侧终止，不等同于 Caddy 网关证书。Caddy 网关模式是首期推荐路径。

## 3. 代码组织和模块边界

### 3.1 后端

- `models/`：持久化实体和关系，模型只负责数据表达，不放跨模块业务逻辑。
- `services/`：节点、隧道、证书、端口、统计和审计服务；通过接口隔离外部依赖。
- `biz/`：面向 RPC/API 的用例编排和参数校验。
- `services/dao/`：GORM 查询、事务和分页；所有新查询补充索引和测试。
- `idl/`、`pb/`：跨 Master/Agent 的稳定协议；字段只追加，不复用已发布字段编号。
- `utils/`：无状态通用工具，避免把业务规则放进 utils。
- `cmd/`：Master、Server、Client、Worker 启动入口。

### 3.2 前端

- `www/pages/`：路由页面，页面只负责组合数据和布局。
- `www/api/`：API/RPC 客户端和类型化请求。
- `www/components/`：领域组件；通用控件放 `components/ui`，领域控件放对应目录。
- `www/hooks/`：React Query 查询、实时订阅和表单逻辑。
- `www/store/`：只保存跨页面的轻量 UI 状态，不把服务端数据复制到全局 store。
- `www/styles/`：设计 token、主题和全局样式。

## 4. 数据模型增量

保留上游用户、节点、隧道、Worker 和统计模型，新增或扩展：

- `node_port_snapshots`：节点、协议、监听地址、端口、进程提示、采集时间和来源。
- `port_reservations`：节点、协议、端口、资源类型、资源 ID、预留状态和释放时间。
- `certificates`：域名/SAN、目标 Server、ACME CA、指纹、签发/到期时间、Caddy 存储标识和状态；首期不保存私钥。
- `certificate_jobs`：申请、续期、撤销和部署任务，含重试次数、错误、开始/结束时间。
- `gateway_routes`：域名、监听端口、Server、FRP vhost 入口、隧道、证书和状态。
- `traffic_daily`：隧道、自然日、入站、出站、采集时间、数据来源；按时区切日。
- `operations_audit`：用户、动作、目标、结果、来源地址和时间。

所有日期范围查询使用半开区间 `[start, end)`；SQLite 开启 WAL，写入任务使用短事务，统计和列表查询使用索引与分页。

## 5. API/RPC 约定

- 所有新增 API 返回统一的错误码和可读错误信息，前端不解析后端错误字符串决定流程。
- 创建/更新隧道先做结构化校验，再保存高级 JSON；结构化字段与原始 JSON 保留来源标记。
- 配置下发使用版本号或配置 hash，Agent 幂等应用；重复推送不能重复创建隧道或证书任务。
- 节点心跳包含版本、架构、能力、延迟、运行状态和端口快照摘要。
- 日志和终端使用 WebSocket/已有 RPC 流，前端订阅离开页面时必须取消；大日志使用游标和窗口限制。
- 远程 Shell 默认关闭，开启后仍需 RBAC、审计和节点能力检查。

## 6. 端口发现和冲突策略

端口状态分为：

1. `observed`：Agent 最近一次通过 `ss -H -ltnup` 发现的监听。
2. `reserved`：平台已有配置或待应用配置预留的端口。
3. `requested`：当前操作准备占用的端口。

应用配置前执行“实时快照 + 平台预留”双检查，并将冲突对象返回 UI。端口扫描不能提供 OS 级锁，最终以 FRP/Caddy 启动时 bind 结果为准。无法读取进程名时不阻塞流程，只显示协议、地址、端口和采集时间。

## 7. UI/UX 和视觉规范

- 首屏直接进入总览，不做营销型 Landing Page。
- 采用浅色/深色主题、低对比中性色背景、单一强调色；状态颜色只用于状态，不大面积铺色。
- 主要容器使用 8px 圆角，弹窗、表单、表格和图表保持一致的间距与边框；不嵌套卡片。
- 使用 Lucide 图标，图标按钮提供 tooltip；文本按钮只表达明确动作。
- 页面布局为固定侧栏 + 顶部上下文栏 + 可滚动内容区；移动端侧栏变为抽屉。
- 列表提供骨架屏、空状态、错误状态、分页/虚拟滚动和键盘焦点；表格列在窄屏按优先级折叠。
- 创建/编辑流程使用抽屉或对话框，保存后保留上下文并刷新受影响的数据，不整页闪烁。
- 对异步操作显示明确的进行中、成功、失败和重试状态；危险操作要求二次确认。
- 实时日志使用等宽字体、限长缓冲和自动滚动开关；终端在 Shell 禁用时显示权限状态而非空白。

## 8. 性能目标

- 页面首次可交互目标：生产构建桌面端不超过 2 秒，移动端不超过 3 秒（基于本地部署和合理节点数量）。
- 页面跳转使用 Next.js 预取和 React Query 缓存，返回列表保留筛选、分页和滚动位置。
- 首屏只加载总览所需数据；图表、Monaco、xterm、Worker 编辑器等重组件按路由动态加载。
- 列表默认分页，服务端限制最大页大小；图表按日聚合，不传输原始流量明细。
- 输入搜索和端口过滤 debounce；轮询采用退避，实时通道优先 WebSocket/RPC 推送。
- 后端 SQLite 开启 WAL、合理索引和连接池上限；配置变更与统计采集分离。
- 生产构建禁止无版本镜像和无界日志；Docker 镜像使用固定版本、最小运行时和健康检查。

## 9. 安全和运维要求

- 节点凭证只显示一次，数据库保存摘要或加密值；支持轮换和立即撤销。
- Caddy 私钥留在目标 Server 的持久化目录，不提供 Master 下载；证书页面展示元数据和部署结果。
- Caddy 配置应用前执行格式检查和冲突检查，失败时保留上一份可用配置并支持回滚。
- 远程终端、证书操作、凭证轮换、配置下发和删除操作写入审计日志。
- Compose 默认挂载最小目录，Worker 不挂载 Docker socket；Shell 开关和 RBAC 在服务端强制执行。
- 备份 SQLite 前执行一致性检查，恢复流程必须有版本和迁移校验。

## 10. 分阶段开发和验证

### 阶段 0：基线导入

- 导入固定上游提交，保留上游许可证和版本信息。
- 安装 Go/Node 依赖，运行 Go 测试、前端 lint/build。
- 记录本地启动方式和最小 Docker 验证步骤。

### 阶段 1：产品化骨架

- 完整移除 WireGuard 导航、页面、API/协议、数据模型、运行时和未使用依赖。
- 增加网络模式、版本、Compose 模板和运行时配置目录。
- 统一前端主题 token、布局、加载/错误/空状态和页面转场。

### 阶段 2：节点和端口

- 节点状态心跳、Compose 生成、凭证轮换。
- Agent 端口快照、Master 端口预留和冲突校验。
- 新增节点详情中的端口、日志、终端和部署信息。

### 阶段 3：隧道和统计

- TCP/UDP/HTTP/HTTPS 表单、高级 JSON 校验和网关路由关联。
- 按日流量表、1/3/7/30 天、自定义日期、时区和历史数据。

### 阶段 4：Caddy 证书中心

- 证书申请/续期任务、HTTP-01/TLS-ALPN-01、Caddy 配置生成和状态回报。
- Server 端持久化、平滑重载、失败回滚，完成 `8880 -> 443` 端到端测试。

### 阶段 5：Worker 和发布

- Linux x86_64/ARM64 Worker 部署、状态、Ingress 和隔离策略。
- 审计、告警、备份恢复、升级文档和发布镜像。

## 11. 当前开发进度

### 已完成

- 导入并固定上游基线，建立产品规划和开发计划。
- 调整总览、导航和节点管理的基础界面；WireGuard 已从默认导航和 Master API 路由中移除。
- 完成 WireGuard 代码清理：删除旧页面、协议生成文件、模型、运行时、文档和未引用的 iptables 依赖。
- Server/Client 凭据弹层提供 Docker Compose 预览和复制；镜像标签取 Master 构建版本，开发构建回退到 `alpha`，容器参数使用数组编码。
- Linux Agent 定时采集 TCP/UDP socket 快照，Master 校验节点凭据后保存最新快照；新增受租户范围保护的 `/ports` 查询页。
- 节点镜像缺少 `ss` 时，端口扫描回退到 gopsutil 的内置连接表读取。
- 证书中心已完成托管证书和证书任务的数据模型、SQLite 迁移、租户隔离 CRUD/API 以及前端登记、编辑、删除和申请入口；DNS 仍由用户手动配置，私钥不回传 Master。
- Server Compose 已包含 host 网络的 Caddy 服务、持久化数据目录和启动占位配置；Server Agent 以只读方式挂载同一 Caddy data 目录，并通过本机 Admin API 和证书扫描接口读取状态；客户端 Compose 保持单容器输出。
- Server Agent 已通过认证 RPC 接收 Caddy 配置，调用本机 Admin API 做适配、校验、热加载和失败回滚；Caddy 的实际证书元数据会回报 Master，Master 不伪造签发结果。
- HTTPS Gateway 已支持域名、监听端口、目标 Server、HTTP 隧道和证书的 CRUD、同步及关联校验；Caddy 反向代理保留原始 Host，支持 `域名:443 -> frps vhostHTTPPort -> Client:8880`。
- 证书的 `http-01`/`tls-alpn-01` 选择会转换为 Caddy ACME issuer 配置，分别关闭另一种挑战方式；DNS 记录仍由用户手动维护。
- TCP/UDP 隧道创建和编辑已接入 frps 保留端口、最新节点 socket 快照和同 Server 已配置隧道的冲突预检；最终 bind 结果仍以 Agent/FRP 应用配置时为准。
- 已在 Ubuntu 24.04 Docker 测试节点完成 Master、Server Agent、Client Agent 和 Caddy 的远端联调；Client/Server 重启后节点、配置和 HTTP/TCP/UDP 隧道均可恢复。
- 远端验证了节点心跳、端口快照、流量统计、实时日志、Compose 参数生成、HTTPS Gateway CRUD/Caddy Admin API 热加载，以及远程 Shell 开关：默认配置可用，设置 `CLIENT_FEATURES_ENABLE_REMOTE_SHELL=false` 后被 Agent 拒绝。
- 远端验证了测试域名经 Caddy、frps vhostHTTPPort 到 Client 的配置链路，Gateway 状态为 `active`；测试环境无公网解析，证书状态保持 `pending`，未将其记录为成功签发。
- 已验证托管证书 `autoRenew=false` 能通过更新接口写入 SQLite；证书私钥仍只保存在目标 Server 的 Caddy 数据目录。
- 已新增历史流量查询 API 和流量页日期筛选，支持 1/3/7/30 天、自定义半开日期区间、按 Client/Server/隧道过滤，并将累计快照转换为每日增量。
- 已完成远端 API 验收：节点状态、端口快照、TCP/UDP 隧道创建与删除、重复端口拒绝、改名改端口、启停和高级 JSON 更新路径均正常；非法高级 JSON 被拒绝且原配置保持不变。
- 已新增节点详情页，客户端和服务端操作菜单可进入同一上下文，集中查看心跳、Agent 版本、Compose、端口快照，并直接打开实时日志和远程终端；服务端日志/终端入口兼容 `serverID` 参数。

### 本轮已完成

- `port_reservations` 使用事务和物理节点唯一索引完成跨请求原子预留；启动回填覆盖 Server 固定监听、活动 TCP/UDP 隧道和 Gateway 端口，Caddy 的 80/443 以共享监听预留。
- Gateway 同一 Server/端口的多域名路由共享一条预留；路由更新和删除按剩余引用数释放端口，并迁移旧的逐路由预留记录。
- 默认 Server 初始化和配置更新会在同一事务中对账 Caddy 与 FRPS 固定端口预留。
- Client、Shadow Client、隧道重建和客户端删除会在同一事务中按剩余活动引用释放 TCP/UDP 预留，改名和端口变更不会遗留旧锁。
### 尚未完成

- Caddy Agent 的配置同步、状态扫描和网关路由链路已经完成；真实 ACME 签发、续期、受浏览器信任的证书链和公网 `80/443` 验收仍需要可解析到测试 Server 的公网域名。
- 当前代码已在远端 Linux Docker 节点构建并运行测试镜像；本机不启动服务。远端已完成滚动升级、SQLite 备份、回滚容器保留和健康检查。

### 下一增量

下一步在具备公网 DNS 的 Linux Docker 环境完成 `8880 -> 443` HTTPS 路由、证书签发和续期状态的真实纵向验收；当前测试节点无公网 IP，真实 ACME 验证待条件具备后进行。

### 最近验证

- `go test ./...` 通过。
- `pnpm lint`、`pnpm exec tsc --noEmit` 和 `pnpm exec next build` 通过。
- Lint 保留若干上游 React Hook 依赖警告；Next.js 提示静态导出不会应用 rewrites，Browserslist 数据库可更新。
- Compose 样例通过 YAML 解析；Master Compose 已在远端实际启动，Server/Client host 网络容器已完成生成参数和滚动部署验证。
- 远端测试服务器（Ubuntu 24.04、内网地址）已完成节点、隧道、端口、日志、统计、Gateway 和 Shell 开关的纵向验收；没有公网 IP，因此跳过真实 ACME 签发与公网 HTTPS 访问验收。API 验收使用临时 TCP/UDP 隧道并已清理。
