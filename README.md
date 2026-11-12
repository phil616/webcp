# WebCP · Web 控制面板

以 OpenResty 容器为内核的独立网站 GUI 管理器。Go 监听端口并内嵌 Vue 界面，一个可执行文件即可提供管理服务，无需 Node.js 运行时。

前端直接复用 [1Panel](https://github.com/1Panel-dev/1Panel) 的主题、布局、抽屉、对话框、表格和编辑器组件，并沿用其操作习惯；SSL 页面和 ACME/DNS 核心代码直接复用固定提交。复用范围和操作约定见 [GUI 说明](docs/gui-1panel.md)。项目采用 GPL-3.0，具体参考范围见 [来源说明](docs/attribution.md)。

## 功能

- **网站**：静态网站、域名/端口/SSL 绑定、运行子目录、默认文档、按路径的单源 HTTP(S) 反向代理、CORS、真实 IP、域名/路径/404 重定向、启用/停用及文件管理。
- **四层代理**：TCP / UDP 单目标转发、IPv4 / IPv6 监听、TLS 透传、超时和 UDP 响应数、向 TCP 后端发送 PROXY protocol；复用 1Panel 表格与抽屉操作。
- **默认页面**：OpenResty 直接提供 403、404、495、496、500、502、503、504 HTML 文件；支持网站覆盖、隔离预览和内容版本保护，编辑即时生效，HTML 不进入 SQLite。
- **Lua**：模块通过 `require("name")` 复用；代码块可跨网站绑定至 access、content、header_filter、body_filter、log 阶段。应用前编译检查。
- **HTTPS 设置**：HTTP 跳转、同时访问、仅 HTTPS；HSTS、TLS 协议/套件、HTTP/2 与 HTTP/3。
- **SSL 证书**：直接复用 1Panel 原版申请、上传、详情、ACME/DNS 账户、自签 CA、续签、日志、下载和删除流程；支持 HTTP/DNS/手动 DNS、目录推送和后置脚本。
- **mTLS**：独立模块管理客户端 CA、客户端证书、吊销/CRL 和网站绑定，复用原版 CA 页面及通用组件，与 SSL 证书命名空间分离。
- **备份**：配置与网站文件完整备份，本地保存、S3 兼容存储上传/列举/恢复，SHA-256 校验、受限解包、中断恢复事务。
- **引擎**：自定义容器名称和镜像，选择已有 OpenResty 容器，备份原主配置后接管；配置预览、性能调节、校验并应用、启动/停止及容器日志。校验失败保留旧配置。

控制台使用注入的访问密钥登录，不引入多用户、RBAC 或商业授权。网站功能不包含伪静态、PHP、网站密码访问、负载均衡、流量限制和运行环境。静态请求只查找实际文件，不进行 SPA fallback 或伪静态重写；PHP 类文件请求直接返回 404，也不会显示源码。

## 安装与发布

当前版本见 [VERSION](VERSION)。二进制包安装、升级和回退见 [安装说明](docs/installation.md)，候选版本检查与打包见 [发布说明](docs/releasing.md)。使用 `./webcp -version` 可在不启动服务的情况下检查构建身份。

## 构建与运行

环境：Linux、Go **1.26.11+**、Node.js **22.12+**（仅构建前端时需要）、Docker CLI 和可访问的 Docker Engine。建议 Docker 为本机守护进程；远程 Docker 必须看到完全相同的宿主机数据路径。

```bash
make build
# 生成只保存在本机配置文件中的随机访问密钥
umask 077
python3 - <<'PYKEY'
import json,secrets
from pathlib import Path
config = Path('webcp.json')
if not config.exists():
    config.write_text(json.dumps({'accessKey':secrets.token_urlsafe(32)},indent=2)+'\n')
config.chmod(0o600)
PYKEY
./bin/webcp -config ./webcp.json
```

打开 **http://127.0.0.1:9080**，使用 `webcp.json` 的 `accessKey` 登录。首次启动不会创建容器或占用网站端口。在界面中创建网站，然后点击「校验并应用」启动 OpenResty。

默认使用 `1panel/openresty:1.27.1.2-5-1-focal`。可预先拉取镜像，或在 Docker 校验时自动拉取。部署目录只需二进制和一个可写数据目录；Web 已内嵌。

```bash
./bin/webcp -config ./webcp.json \
  -listen 127.0.0.1:9080 \
  -data /var/lib/webcp \
  -container webcp-openresty \
  -image 1panel/openresty:1.27.1.2-5-1-focal \
  -http-bind 0.0.0.0:80 \
  -https-bind 0.0.0.0:443
```

访问密钥也可通过 `WEBCP_ACCESS_KEY` 或 `WEBCP_ACCESS_KEY_FILE` 注入；未配置密钥时拒绝启动。登录会话有效期 12 小时，退出或重启服务后失效。容器选择、接管与密钥配置见 [访问与容器配置](docs/webcp-access-runtime.md)。默认绑定回环地址；远程操作可通过 SSH 端口转发：

```bash
ssh -L 9080:127.0.0.1:9080 user@server
```

网站和四层代理端口由 Docker 发布，管理端口由 Go 监听。发布端口变化时，应用配置会自动重建托管容器；切换失败会恢复旧容器和配置。此过程会短暂中断网站与代理连接。旧版本容器也会在首次应用时重建一次，以更新端口管理方式。更换镜像仍需停止并删除**此管理器创建的**容器后重新应用，数据目录保留。不要将不同数据目录指向同一个容器名。

网站设置与 1Panel 行为对应见 [网站设置说明](docs/website-settings.md)；四层转发见 [四层代理说明](docs/l4-proxy.md)。

## 使用顺序

1. 创建静态网站或配置一个代理源站；域名使用小写 ASCII、punycode 或 IP。
2. 在「SSL 证书」申请、上传或自签服务器证书，再到网站「HTTPS」设置中绑定。申请流程与 1Panel 一致；网站证书必须覆盖所有绑定域名。
3. 如需 mTLS，在独立「mTLS」模块创建或导入客户端 CA、签发客户端证书，再到该模块「网站绑定」启用客户端验证。
4. 在 Lua 资源库创建模块与代码块，在网站配置中按阶段绑定代码块。
5. 保存配置。OpenResty 运行时自动校验并应用；四层监听变化会重建容器，其余变更使用重载；停止时保存草稿，通过状态条「启动」或「校验并应用」。网站文件保存即时生效。
6. 在备份页配置 S3，测试权限后创建完整备份。空 Secret Key 表示保留原密钥。

SSL 下载为含证书和私钥的原版 ZIP；mTLS 客户端「下载」导出含私钥的 PEM 凭据。客户端可这样测试 mTLS：

```bash
curl --cacert server-ca.pem --cert client-bundle.pem --key client-bundle.pem https://example.com/
```

如果使用公网签发的服务器证书，可省略 `--cacert`。客户端 CA 与服务器 CA 可以不同。

GUI 吊销客户端后，运行中的 OpenResty 会自动应用新的 CRL；直接调用 API 后需应用配置。mTLS 关闭会话缓存和票据，已有长连接不会被主动中断。自建证书有效期不会超过签发 CA，CRL 有效期与 CA 对齐，每次应用重新生成。到期证书不会阻止管理器启动，但不能部署绑定到到期证书的启用网站。

## 数据与恢复

```text
data/
  webcp.db                   # SQLite 控制状态、配置、证书、连接和备份索引，0600
  webcp.db-wal / webcp.db-shm # SQLite WAL 运行文件
  cache/<website-id>/        # 代理缓存，可重建，不进入备份
  manager.lock               # 单进程独占锁
  sites/<website-id>/        # 网站文件，与容器只读共享
  ssl/logs/                  # SSL 申请/续签日志
  runtime/
    acme/                    # HTTP-01 临时挑战，只读挂载给 OpenResty
    root/default/             # OpenResty 全局错误页面文件
    root/sites/<website-id>/  # 网站独立错误页面文件
    current -> generations/<id>
    generations/<id>/        # 不可变 Nginx、Lua、部署证书
  backups/<id>.tar.gz         # 完整备份
  state.json.migrated         # 首次迁移保留的旧配置原件，后续不读取
```

SQLite 和页面文件的职责、迁移及恢复机制见 [持久化说明](docs/persistence.md)。旧 JSON 首次启动自动迁移，原件保留。新备份分别包含配置导出、网站文件和页面文件；兼容旧 schema=1 备份恢复。

OpenResty 容器只读挂载 `runtime` 和 `sites`，并单独可写挂载 `cache`；不挂载管理状态、备份或 S3 密钥。容器以管理器的 UID/GID 运行，根文件系统只读，临时文件写入 `/tmp`。

恢复会停止 OpenResty，验证备份摘要并解包到独立目录，再替换网站文件与配置。恢复后保持停止状态，需检查并应用配置重新启动。恢复包含四层代理及其监听端口、证书、Lua、错误页和 S3 凭据，但不包含容器名、镜像、HTTP/HTTPS 发布地址等启动参数。恢复中断时，下一次管理器启动会按照日志回滚到恢复前状态。请保留整个数据目录，勿手动删除恢复日志。

本地和远程备份均可从 GUI 恢复。S3 桶需事先创建，连接测试会验证写入、回读、列举、删除权限。备份包含私钥和访问凭据，使用私有存储桶；备份期间暂停外部部署写入。当前提供手动完整备份，不做定时或增量备份。

## 开发与验证

```bash
make test          # Go 单元测试 + race
make integration   # 真实 OpenResty + MinIO Docker 集成测试，自动清理测试容器
make check         # 来源/文档/版本、Go race/vet、前端格式与完整生产构建
```

前端开发：先构建一次，然后分别运行 `make dev CONFIG=/absolute/path/webcp.json` 和 `npm run dev --prefix web`，访问 `http://127.0.0.1:5173`。Vite 将 `/api` 转发到 `127.0.0.1:9080`。上游和本地 Vue 源码统一编译、完整类型检查，生产构建通过 `go:embed` 嵌入 Go。流程与框架机制见 [前端工作流](docs/frontend-workflow.md)。

浏览器测试及覆盖范围见 [测试说明](docs/testing.md)；分层、并发模型、配置事务和扩展约束见 [架构说明](docs/architecture.md)；HTTP 端点见 [API 说明](docs/api.md)。

SSL 和 mTLS 的源码复用清单、模块边界及迁移说明见 [证书模块说明](docs/certificates.md)。

容器选择、接管前备份、密钥配置与优先级见 [WebCP 访问与容器配置](docs/webcp-access-runtime.md)。
