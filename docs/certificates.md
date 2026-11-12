# SSL 与独立 mTLS

## 复用基线

固定使用 [1Panel 提交 3adee9445897aa41c0160b4f805a9b1f43f712ac](https://github.com/1Panel-dev/1Panel/tree/3adee9445897aa41c0160b4f805a9b1f43f712ac)。SSL 页面直接使用该版本 `frontend/src/views/website/ssl` 的源码；网站 HTTPS 设置直接使用 `website/config/basic/https` 及其 `components/https`。不是重新绘制的近似页面。

`web/panel/src` 包含原版证书列表、申请、上传、详情、申请确认、手动 DNS、ACME 账户、DNS 账户、CA 创建/上传/详情，以及实际使用的 LayoutContent、ComplexTable/FuTable、DrawerPro、DialogPro、RouterButton、OpDialog、CopyButton、日志、文件选择器、CodeMirror、校验规则、中文语言包和主题。`web/panel/manifest.json` 记录 148 个上游源文件的提交、Git blob、原始/适配后 SHA-256 和逐文件补丁。导入路径的命名空间调整也在补丁中。

```bash
python3 scripts/vendor-1panel.py --verify
# 从固定提交及已审阅补丁重建相同文件；不会重新设计组件：
python3 scripts/vendor-1panel.py --upstream /path/to/1Panel
```

只有本程序的 HTTP 传输、JSON 持久化、单实例路由/状态桥接及 mTLS 专有功能是本地实现。原版表格的权限判断被移除；面板账户、授权、商业版、节点、跨节点同步和完整文件管理器的收藏入口不属于独立程序范围。保留原版 SSL 可见界面和操作流程，不加入本地的客户端证书选项。mTLS CA 管理复用同一个原版 CA 页面，客户端签发、吊销和绑定使用相同原版抽屉、确认框和密钥选项。

## SSL 功能与上游代码

- ACME：Let's Encrypt、ZeroSSL、Buypass、Google Cloud、自定义 ACME/EAB；复用 `agent/utils/ssl/acme.go`、`client.go`、`manual_client.go`。
- 验证：HTTP、DNS 账户、手动 DNS；复用上游 `dns_provider.go` 的全部提供商工厂、DNS/CNAME/自定义解析服务器选项，以及 1Panel 的 lego v5 分支。
- 证书：原版申请状态、错误信息、日志、自动续签开关、到期处理、备注、详情、更新、下载、删除与 ACME 吊销。SSL ZIP 内容与原版一致，为 `fullchain.pem`、`privkey.pem`。
- 自签 CA：复用上游的密钥生成、叶证书 KeyUsage、CA ZIP/PEM 校验函数；支持原版 EC/RSA 选项、签发及续签。CA ZIP 内容为 `ca.crt`、`ca.key`，支持粘贴、宿主机文件和 ZIP 导入。
- 部署：续签保持证书记录 ID 和网站绑定；推送本地目录、执行显式配置的后置脚本、更新使用该证书的网站。脚本超时 30 分钟，与原版一致；工作目录为数据目录，或配置的第一个推送目录。
- HTTPS：原版已有证书/手动导入、HTTP 访问方式、HSTS/子域、TLS 协议/套件和 HTTP3 表单。HTTPS 端口取实际运行参数。证书选择只读取 SSL 命名空间。

自动续签在服务启动及每小时扫描；普通证书提前 30 天，短有效期证书按有效期三分之一处理。手动上传、手动 DNS 不自动续签。申请以后台任务运行；重启后未完成的任务显示中断状态，可以重新申请。ACME 代理使用进程的 HTTPS_PROXY/HTTP_PROXY，替代完整面板的全局代理设置。

上游核心的修正包括：CodeMirror 只在挂载后创建一次实例；手动 DNS 使用同一 `golang.org/x/crypto/acme` 依赖的 `CreateCertFromOrder`，兼容完成订单响应不带 Location 的 RFC 8555 CA。证书序列号使用随机 128 位，防止并发签发碰撞。前端另修复 DialogPro 打开事件、抽屉卸载时全屏状态清理、HTTPS 取消/失败后状态恢复、申请失败后加载状态清理及类型契约。菜单增加折叠状态下的可访问名称。这些修改均保留补丁，未另写 ACME 协议实现。构建、布局及共享状态机制见 [前端工作流](frontend-workflow.md)。

## 数据边界

| 数据 | 归属 | 依赖 |
| --- | --- | --- |
| `state.certificates` | SSL 服务器证书和 SSL CA 的实际 PEM | SSL 元数据引用 materialId |
| `state.ssl` | 原版 SSL/CA/ACME/DNS 元数据、续签与部署参数 | 仅引用 SSL 证书材料 |
| `state.mtls.authorities` / `cas` | mTLS CA 材料 / 原版 CA 页面元数据 | 独立命名空间 |
| `state.mtls.clients` | 客户端证书和永久保留的吊销记录 | 仅可引用 mTLS CA |
| `state.mtls.bindings` | 网站与客户端信任 CA 的绑定和开关 | 引用网站 ID、mTLS CA ID |
| `site.tlsId` / `https` | 网站的 SSL 服务器证书与 HTTPS 参数 | 不保存客户端 CA |

SSL CA 与 mTLS CA 可以具有相同数值 ID，但互不引用。不能把 SSL CA ID 直接用于客户端签发；需要在 mTLS 模块显式创建或导入 CA。删除 SSL 证书/CA 不会修改 mTLS 数据。

mTLS 启用前，网站必须已启用 HTTPS，且 HTTP 模式为自动跳转或禁止 HTTP。独立绑定验证阻止通过 HTTPS 设置意外关闭仍在使用的 mTLS。删除网站会清理其 mTLS 绑定。客户端只签发 clientAuth；服务器证书使用 serverAuth。CA 必须能签署 CRL。

部署时分别生成 `certs/` 和 `mtls/`，mTLS 使用独立 CA 与 CRL，强制客户端验证，并关闭会话缓存与票据。CA 私钥、客户端私钥不挂载进 OpenResty。客户端签发记录不提供单独删除操作；吊销记录保留至对应 CA 退役。已建立连接不受新 CRL 影响。

## 迁移与备份

现有 schema-1 配置和旧备份在加载/恢复时规范化：客户端证书移入 mTLS；历史双用途 CA 在 mTLS 命名空间保留独立副本；原网站 clientCaId 转成独立绑定。保留原证书、私钥、序列号、有效期和吊销时间，不重新签发或恢复已吊销凭据。再次加载不重复迁移。

完整本地/S3 备份包含从 SQLite 导出的 schema=2 state.json、网站目录及独立 OpenResty 页面文件，因此覆盖 SSL 账户、DNS 凭据、SSL 与 mTLS 证书、绑定和续签配置。日志、ACME 临时挑战、代理缓存不属于备份。overview 和证书列表隐藏私钥和账户密钥；显式详情、下载和 DNS 账户编辑遵循原版管理语义。

## 运行验证

`go test -race ./...`、`go vet ./...`、前端类型检查/构建，以及 Playwright 操作回归覆盖模块分离、旧数据迁移、CA 签发/导入、HTTPS 与 mTLS 绑定、客户端吊销、原版 SSL 表单及下载。

真实 Docker 集成覆盖客户端有/无证书访问、吊销后拒绝访问。另有独立 [Pebble](https://github.com/letsencrypt/pebble) 测试，实际经过 OpenResty 验证 HTTP-01，并验证续签、手动 DNS-01 和 ACME 吊销；不会向公网 CA 或 DNS 服务商写入。

```bash
docker pull ghcr.io/letsencrypt/pebble@sha256:0c81f05db371f436c97fb894bd6d2ab8fc9c8e2ce40f8bef8a8e15941bcd75ef
RESTY_ACME_INTEGRATION=1 go test -v ./internal/service -run '^TestSSLACMEIntegration$' -count=1
```

DNS 提供商凭据由上游构造器校验。各供应商的公网申请需要用户的实际账户与域名；测试不代替真实供应商凭据验证。

## mTLS 证书错误页面

全局“默认页面”和网站“错误页面”均支持 495（客户端证书校验失败）及 496（未提供客户端证书）。网站配置覆盖全局配置。仅在启用 mTLS 的 HTTPS 虚拟主机内生成对应规则，通过内部页面返回 HTTP 403，保留证书验证和 CRL 校验。实际容器验证覆盖无证书、错误 CA 签发的证书、合法证书与吊销。浏览器对服务器证书不信任等 TLS 握手失败发生在 HTTP 页面之前，不属于这两个页面的处理范围。参见 [NGINX SSL 错误处理](https://nginx.org/en/docs/http/ngx_http_ssl_module.html#errors)。

Nginx 主配置发现还复用 `agent/utils/nginx` 的 lexer、通用 AST 和 Lua 代码块解析，位于 `internal/onepanel/nginx`；去掉未使用的面板专用 server/location/upstream 包装，来源和补丁包含在同一复用清单中。
