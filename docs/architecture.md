# 架构与维护约束

## 边界

```text
Vue views/components → HTTP API → service.Manager → domain + store
                                          ├→ openresty.Runtime → Docker CLI
                                          ├→ pki              → crypto/x509
                                          └→ backup           → tar/gzip + MinIO SDK
```

- `internal/buildinfo`：非敏感的构建身份，由统一 VERSION 与构建脚本注入，支持 CLI 和登录后查询。
- `cmd/web-panel`：启动参数、数据目录独占锁、依赖组装、优雅退出。不放业务逻辑。
- `internal/domain`：配置模型、引用完整性和输入约束。无全局状态，不依赖 HTTP、Docker 或 SDK。
- `internal/store`：SQLite 迁移、独立资源仓储、事务及 revision 比较交换；不管理容器或 HTML 内容。
- `internal/errorpages`：OpenResty 页面文件、内容 ETag、原子目录交换及首次资源初始化。
- `internal/fsutil`：文件持久化原语，供迁移和文件适配器使用。
- `internal/service`：统一写入入口，协调草稿、网站文件、证书、备份与恢复。按能力拆为多个文件。
- `internal/openresty`：纯文件生成与容器控制分别实现。Runtime 接口允许测试替身，生产只使用 Docker 实现。
- `internal/pki`：类型明确的 CA、服务器、客户端用途；证书链、私钥匹配、有效期、CRL。
- `internal/backup`：版本化格式、严格解包与 S3 传输，不依赖业务服务。
- `internal/httpapi`：路由、严格 JSON、If-Match、响应和跨站请求保护。统一入口执行密钥会话认证。
- `web/panel`：原版 SSL 及 HTTPS 页面、核心组件与独立构建适配；固定来源、逐文件补丁和哈希验证。
- `web/src/vendor/1panel`：固定版本的 1Panel 主题和组件，逐文件记录来源和修改。
- `web/src/components/panel`：导航、表格、抽屉和领域表单适配，不维护另一套通用控件。
- `web/src/views`：按功能分开的页面；api.ts 统一处理版本、错误及运行时自动部署。无远程 CDN、演示数据、模拟成功返回。

## 持久化与并发

控制状态持久化到 data/webcp.db。网站、L4、Lua、证书、SSL/mTLS 分资源表保存，独立表保存性能参数、S3、机器本地运行连接、备份索引与恢复日志。嵌套配置使用类型受限的 JSON 列；不使用万能键值表或完整状态 JSON 作为主存储。State 是领域聚合和便携导出，不是数据库存储布局。

数据库启用版本化 SQL 迁移、STRICT 表、主键及 JSON 检查、WAL、FULL 同步、5 秒 busy_timeout。写连接上限 1，只读连接上限 4，读事务与写事务分离。配置写入在事务内比较 revision 并提交全部相关资源，失败完整回滚。启动先取得数据目录 flock，再打开和迁移 SQLite。未来版本数据库拒绝加载。

HTML 不存在于 State、Site 或数据库。默认页面权威文件在 runtime/root/default，网站覆盖在 runtime/root/sites/<id>。页面通过独立文件接口和内容 ETag 更新，不递增配置 revision，不生成部署版本，也不触发重载。网站普通文件同样独立于配置事务。

Manager 继续串行协调配置、部署及备份，保证管理器自身的一致性；ACME 网络签发在锁外运行。耗时部署和备份仍可能占用业务锁，本次没有引入多节点或后台任务调度。详细数据库布局、旧数据迁移、页面归属和运行边界见 [持久化说明](persistence.md)。

## 配置部署

1. 从同一 revision 渲染独立 generation，固定容器路径，输出对页面文件的引用。
2. 域名、上游地址和 ID 先校验。网站配置文件编辑复用 1Panel 解析器，转换为类型化 GUI 数据后重新生成并比对完整 AST；不能完整表达的改动拒绝保存，不持久化原始 Nginx 文本。Lua 写入部署文件；HTML 由 OpenResty 固定文档目录提供，不写入配置或 generation。
3. 使用实际 OpenResty 镜像执行 `nginx -t`；对每个 Lua 模块和代码块执行 LuaJIT 编译。
4. 校验成功后，用原子 symlink rename 切换 current，再 signal reload；未运行则启动容器。
5. 命令失败时恢复旧 current；旧 generation 保留供旧 worker 使用。渲染失败的目录删除；Docker 操作失败的 generation 保留，避免连接中断造成重载结果不确定时误删 worker 仍引用的文件。

网站文件不属于 generation，更新原子替换目标文件后立即可见。删除网站草稿时保留文件目录，避免未应用的变更破坏当前服务。旧 generation 和已删除网站的目录暂不自动清理，运维应定期核对保留空间；不能直接删除仍被 worker 使用的 generation。

容器必须带匹配数据目录的 ownership label，管理器拒绝停止或重载同名外部容器。使用 CLI 参数数组，完全不通过 shell 拼接命令。镜像在创建时固定；发布端口与草稿不一致时，通过下述切换事务更新。

容器使用本机管理器的 UID/GID，capabilities 全部移除，禁用权限提升，只读根文件系统、runtime 和网站数据挂载，独立 cache 挂载可写。运行时不挂载 SQLite 数据库、WAL、密钥配置和 backups。Lua 是可信管理员代码，具有工作进程的执行能力；本系统不提供多租户隔离。

## 四层代理与容器切换

`domain.StreamProxy` 是单监听、单目标模型，不存在 upstream 池或负载均衡。`stream.go` 在 http 外生成独立 stream 块，排序规则 ID 以确定容器内部端口；内部端口从 10000 分配，宿主机监听地址和端口通过独立 PortBinding 发布。不能把宿主机 IP 直接写入容器 listen。停用规则不生成指令或发布端口。域名在配置应用时由 OpenResty 解析，更新 DNS 后需重新应用。

Docker 不支持对已创建的容器增加发布端口。目标、超时或 PROXY protocol 更新沿用配置校验/原子链接/重载；发布集合或内部端口映射变化时执行替换：

1. 临时容器校验新配置和 Lua，再 create 候选容器；此时旧容器继续提供服务。
2. 原子写入 runtime/replace-journal.json，记录完整 Docker ID、管理容器名、旧 current 和运行状态。
3. 停止旧容器并改名，切换 current，将候选改为托管名称后启动。
4. 检查新容器运行状态及 nginx -t，删除日志提交，再删除旧容器。
5. 提交前失败，以独立于请求取消的上下文移除候选、恢复旧名称/current/运行状态。进程中断时，启动和下一次应用先按日志回滚。恢复失败保留日志并报错；不会移除缺失旧容器时仍存在的新容器。

日志身份使用 Docker ID，所有查询与删除仍检查数据目录 ownership label。旧格式容器在首次应用时替换为 spec=3，提供 host.docker.internal 映射和独立缓存挂载。切换会中断现有网站和代理连接，GUI 明确显示此行为。事务针对进程中断恢复，不承诺断电时 Docker 与文件系统的跨系统原子性。具体使用限制见 [四层代理说明](l4-proxy.md)。

## 网站设置

模型与兼容默认值位于 domain/website.go，输入验证位于 website_validate.go。service/website.go 以 updateWebsite 统一处理独立分区和规则命令；HTTP 只解码各分区 DTO，不能用跨域请求误写证书字段。Manager.mutate 在落盘前统一执行领域验证、网络发布验证与 revision 递增。

website_ports.go 统一规划域名端口、HTTPS/QUIC 及与 L4 的发布冲突。HTTP 自定义内部端口从 20000 分配，与 L4 从 10000 分配的端口区间隔离。HTTP 生成分为站点组织（website_render.go）和具体指令（website_directives.go）。纯渲染可接受 HTTPS 发布端口选项，使预览与部署使用同一重定向逻辑。

网站前端位于 components/website，按域名、目录、文档、代理、CORS、HTTPS、真实 IP、重定向拆分；useWebsiteSection 统一处理 revision 和自动部署。保存期间禁用切换分区；轮询不会覆盖表单草稿。

原单源代理使用缺省字段迁移，仅在 proxies 缺失时生成 default；显式空数组不会重新生成代理。旧 schema-1 配置和备份通过显式迁移进入 SQLite，并将页面 HTML 分离为文件。可再生成的代理缓存位于 data/cache，不属于备份内容；缓存参数属于配置。完整行为和边界见 [网站设置说明](website-settings.md)。

## SSL 与 mTLS

SSL 元数据复用 1Panel 模型，ACME/DNS 和 CA 纯函数位于 internal/onepanel；不引入 GORM、面板容器、节点、RBAC 或授权服务。服务分为 ssl.go（记录/导入/删除）、ssl_accounts.go、ssl_ca.go、ssl_download.go、ssl_jobs.go、ssl_proxy.go。网络签发在互斥锁外运行，只有状态提交持锁；同一个证书禁止重复申请，原版 lego 全局日志/解析配置串行使用。后台任务使用管理进程的取消上下文，重启后恢复中断状态。

mTLS 数据和服务独立，CA 密钥生成/解析与通用页面复用，不共享 SSL 的证书存储、签发账户或绑定。网站 HTTPS 不保存客户端 CA；部署时从独立绑定投影到运行配置。旧格式在配置加载和备份恢复时幂等迁移，保留吊销记录。

完整字段、依赖方向、迁移和运行验证见 [证书模块说明](certificates.md)。

## 备份与恢复事务

格式为 gzip 压缩 tar，包含从 SQLite 一致性读取的 schema=2 state.json、sites/<id>/... 和 openresty/root 下的独立页面文件；配置导出不含 HTML。本地摘要及备份索引保存到 SQLite，S3 对象元数据保留 SHA-256。不复制正在运行的数据库或 WAL 文件。旧 schema=1 备份在解包时将内联页面迁出。

解包仅接受允许路径内的普通文件和目录，拒绝路径穿越、链接、重复成员、特殊文件和未知网站目录；检查大小、条目数、页面状态码与每页 256 KiB 上限。总体限额继续为 2 GiB、单文件 128 MiB、100000 个条目。

恢复完成验证后停止 OpenResty，在 SQLite 保存恢复操作和旧配置，再切换网站和页面目录，以事务提交新配置并移除恢复操作。提交前失败或中断时恢复两个旧目录及原配置。恢复完成后保持停止，等待检查和应用。目录交换与 SQLite 之间通过恢复协议协调，不声称两者具有一个跨系统 ACID 事务。

管理器操作互斥不覆盖外部部署写入；备份期间需暂停外部文件部署。S3 使用官方 SDK，TLS 验证保持开启。具体迁移、恢复步骤与测试见 [持久化说明](persistence.md)。

## 后续修改原则

新增功能先扩展领域模型及 schema 策略，再实现服务，最后增加 HTTP 和界面；不在 handler 内写文件或生成 Nginx。不增加万能 JSON 配置表或任意指令编辑器。涉及部署、引用、私钥、解包和恢复事务的修改，需要覆盖成功与失败路径；无需给纯样式写镜像测试。

前端复用范围和操作约定见 [GUI 说明](gui-1panel.md)。

本项目优先复用 1Panel 已有实现：新增需求先定位固定上游源码，已有页面、组件、校验和协议核心应直接复用，通过明确的传输/持久化适配层接入。只有上游不存在或不支持的能力才新增本地实现。不要在全局样式中重绘或覆盖原版组件；更新上游时一起审阅固定提交、逐文件补丁和哈希清单。

## WebCP 访问与运行目标

`internal/config` 负责启动配置和密钥注入，`internal/httpapi/auth.go` 在统一 API 入口执行会话认证；业务服务不持有访问密钥。前端 `auth.ts` 和根 App 管理会话状态，`ManagerApp` 只在登录后挂载，401 自动回到登录。

`openresty.Controller` 通过 SQLite 运行连接仓储保存机器本地运行目标并委托托管 Docker 或已有容器部署。已有容器模块按发现、文件传输、配置备份/恢复、部署和文件同步拆分。配置读取直接复用 1Panel 的 Nginx lexer/AST，保留 Lua 代码块边界，避免误读 Lua 字符串中的指令；排除不使用的面板站点语义包装。接管先备份原主配置，校验候选版本后再写入口并重载，恢复日志在提交后移除。容器身份绑定 Docker ID。
