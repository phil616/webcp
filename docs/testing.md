# 验证与开发

## 推荐验证入口

```bash
make check                       # 日常 CI：来源、文档、版本、格式、前端、Go test/vet
make test                        # 按需：Go race 检查
make security                    # 依赖变化：Go 可达漏洞和 npm 依赖审计
make integration                 # 部署/存储/证书变化：全部容器集成，包含 ACME
make test-web                    # 界面变化：生产浏览器回归，自动构建嵌入后端
make test-web-dev                # Vite 调试：同一用例检查 Vue 运行时警告
```

PR 和 main push 仅执行 `make check`。Actions 手动勾选 extended 时额外执行安全扫描、容器集成与生产浏览器回归。无需每次运行开发/生产两套相同浏览器用例；源码重建核对只在需要检查可复现性时执行，见 [发布说明](releasing.md)。

自动浏览器入口为默认推荐方式：随机选择回环端口、生成一次性密钥、创建临时数据目录，并按该目录的 ownership 标签清理自身容器。下文手动启动仅用于调试，不能指向真实数据。

## 常规检查

```bash
npm ci --prefix web
npm run build --prefix web
go test ./...
go vet ./...
npm run format:check --prefix web
```

Go 测试包括引用保护、乐观并发、失败部署不改变版本、PKI 用途隔离与吊销签名、归档路径穿越/链接/重复/超限、完整备份恢复、中断回滚、文件路径隔离、密钥会话与未登录拒绝、跨站请求拒绝与未知字段拒绝。

## 实际 OpenResty、S3 和 ACME

需要本机 Docker。测试使用临时数据目录、随机容器名和回环随机端口，退出清理自身容器，不操作其他容器。

```bash
make integration
```

实际验证：

- HTTPS + mTLS，合法客户端可访问，未提供客户端证书被拒绝。
- 复用 Lua 模块，access 阶段执行并返回响应头。
- 静态文件、自定义 404、代理自定义 502、PHP 源码拦截。
- 无效 Lua 应用失败，current 和旧站点继续可用。
- 吊销客户端并重载 CRL，新的连接拒绝该客户端。
- TCP/UDP 二进制转发、相同宿主端口按协议分别发布、mTLS 经 TCP 透传。
- 目标/超时更新仅重载、真实 TCP PROXY protocol 报文头、监听变更重建、端口被占用时保留旧容器和旧版本、移除 TCP 后 UDP 仍可用。
- 新容器已启动但未提交时模拟进程中断，通过日志恢复旧容器/current，重复恢复无副作用。
- 运行目录与默认文档顺序、路径代理/正则代理/Host、WebSocket 升级、SNI 与上游证书验证。
- 独立缓存命中和清理、HTML 内容替换、CORS 预检/来源/错误页、受信任与非信任真实 IP。
- 域名/路径/404 重定向和路径/查询保留、同域名不同端口网站、HTTPS 三种访问模式、HSTS 与 HTTP/3 发布/Alt-Svc、完整网站设置备份恢复。
- 无效 PCRE 校验失败保留旧部署。
- S3 连接权限、备份上传、远程列表摘要和远程恢复。

OpenResty 镜像为 `1panel/openresty:1.27.1.2-5-1-focal`。S3 测试使用固定摘要的官方 `rustfs/rustfs:1.0.5` 镜像及临时内存目录，仅对这个隔离实例关闭磁盘拓扑检查；不改变 WebCP 的 S3 客户端或用户配置。MinIO 原测试镜像已经无法公开拉取，不再依赖本机缓存。首次测试按需拉取镜像，拉取失败会正常报错。ACME 使用固定摘要的 Pebble，单独调试可执行 `make integration-acme`，无需再与 `make integration` 同时运行。

## 浏览器操作

在一个单独终端启动隔离管理器；浏览器测试会创建网站、证书和备份，不应指向生产数据目录。

```bash
python3 - <<'PY'
import os, secrets
with os.fdopen(os.open('/tmp/webcp-browser-test-key', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as key:
    key.write(secrets.token_urlsafe(32) + '\n')
PY
WEBCP_ACCESS_KEY_FILE=/tmp/webcp-browser-test-key ./bin/webcp \
  -listen 127.0.0.1:19080 \
  -data /tmp/resty-browser-test \
  -container resty-browser-test \
  -http-bind 127.0.0.1:18080 \
  -https-bind 127.0.0.1:18443
```

另一个终端运行：

```bash
cd web
npx playwright install chromium     # 仅浏览器尚未安装时
WEBCP_TEST_ACCESS_KEY="$(cat /tmp/webcp-browser-test-key)" npm run test:e2e:dev             # 自动启动 Vite，代理到隔离后端
WEBCP_TEST_ACCESS_KEY="$(cat /tmp/webcp-browser-test-key)" npm run test:e2e                 # 访问嵌入最新前端的隔离 Go 程序
```

测试使用真实 API；HTTPS 错误恢复用例单次拦截保存请求，验证失败后重试。开发模式同时将 Vue 运行时警告和浏览器异常视为失败。浏览器用例覆盖 1Panel 抽屉/表格、表单校验、文件编辑/上传、独立网站设置、Lua 复用、CA/服务器/客户端签发、mTLS/Lua 绑定、错误页及版本冲突、备份及输入确认、自动重载及无效 Lua 保留旧部署、四层代理创建/编辑/协议切换/地址校验/运行中启停与实际端口发布同步/删除、域名端口/运行目录/默认文档、CORS/真实 IP/并发保护、代理缓存/替换/预览/启停确认、三类重定向、HTTPS 模式/HSTS/HTTP3/停用确认、折叠菜单、深色主题和手机抽屉。自动重载用例启动隔离的 OpenResty 并以实际网站 HTTP 请求验证结果，结束后停止容器。网站端口默认 18080，可用 RESTY_TEST_WEBSITE_URL 指定。设置 `RESTY_TEST_URL` 可指定隔离测试服务地址。测试过程截图输出到 docs/workspace-desktop.png 和 docs/workspace-mobile.png。

## 限制

- nginx -t 与 LuaJIT 编译只证明配置和语法可加载，不验证 Lua 外部依赖、业务逻辑或上游可用性。
- Docker reload 命令成功是部署命令的完成边界；运行中站点的持续健康由容器日志和业务请求检查。
- HTTP/3 集成覆盖 QUIC 配置校验、UDP 发布与 Alt-Svc；尚未运行端到端 HTTP/3 客户端测试。
- ACME 使用可选的固定 Pebble 镜像验证 HTTP-01、续签、手动 DNS-01 和吊销，命令见 [证书说明](certificates.md)。不会自动向公网 CA 或 DNS 服务商写入。
- 不实现定时备份、数据库和运行环境。
- 当前支持 Linux。宿主机二进制需有 Docker CLI 和守护进程操作权限；无须在宿主机安装 Nginx 或 OpenResty。

证书回归还覆盖原版 SSL 申请/上传/详情/下载、ACME 与 DNS 表单、独立 mTLS CA/客户端/绑定、手机抽屉和旧格式迁移；截图见 ssl-certificates.png、ssl-acme-account.png、mtls-clients.png、mtls-bindings.png。

框架回归另覆盖浏览器历史与高亮同步、原版路由/侧栏过渡、SVG 渐变、系统主题、自动折叠、手机卡片操作、HTTPS 失败重试和文件选择器。截图见 framework-desktop.png、framework-mobile.png；构建机制见 [前端工作流](frontend-workflow.md)。

导航与性能回归覆盖网站/mTLS 选中项、前进/后退、刷新与 CA 别名，原版性能表单校验/保存/单位保留/并发冲突/重新加载，以及 495/496 全局与网站覆盖编辑。实际 OpenResty 集成还验证无证书及错误 CA 的客户端返回自定义 HTTP 403 页面、合法客户端访问和运行中 gzip 调节生效。备份恢复检查性能参数及证书错误页面保留。

访问控制用例检查未登录 API 拒绝、错误密钥、Cookie 会话、刷新保留登录、退出撤销会话、401 返回登录页，以及旧会话迟到的响应不会撤销新登录。容器用例覆盖自定义容器名/镜像的保存及独立版本保护。实际 Docker 集成包含无 WebCP 标签的已有容器，以及无预挂载的普通 OpenResty：原配置备份、接管、即时文件同步、HTTP-01 验证文件同步/清理、停止后重启且容器 ID 不变。

SQLite 与文件边界回归包括旧 JSON 迁移和原件保留、WAL 已提交快照读取、跨连接 revision 冲突、SQL 写入失败回滚、页面 ETag/外部编辑/删除后重启，以及恢复中断时同时回滚 SQLite 与页面目录。实际容器回归检查页面即时更新不改变 generation，默认页面和网站覆盖通过独立文件 API 校验。详见 [持久化说明](persistence.md)。
