# 开发与维护

WebCP 为 Linux 上运行的独立 Go 程序，前端统一使用 Vue、Vite、Element Plus 和固定版本的 1Panel 源码。当前版本见根目录 VERSION，最低 Go 工具链见 go.mod；构建前端需要 Node.js 22.12+，推荐 24。

## 开发入口

```bash
make build
make run CONFIG=/absolute/path/webcp.json
make dev-web
```

单独运行 Vite 时默认代理到 127.0.0.1:9080，可通过 RESTY_DEV_API 指定隔离后端。密钥通过配置或环境注入，禁止提交真实配置、数据库、证书、备份和 node_modules。

## 修改边界

- domain 定义业务配置、引用完整性及迁移语义，不依赖 HTTP 和容器控制。
- store 管理 SQLite 仓储及版本化 SQL；配置提交必须经过事务和 revision 检查。
- errorpages 直接编辑 OpenResty 页面文件；HTML 不加入 State、Site 或数据库。
- service 协调业务操作、部署和恢复；handler 负责协议转换，不生成 Nginx 或直接改数据库。
- openresty 按渲染、容器发现、文件同步、部署和恢复拆分。
- 前端优先复用 1Panel 已有组件与交互，通过 adapter 连接本地接口，不增加另一套通用控件。

上游来源和补丁在 web/panel/manifest.json，修改复用文件时同步更新对应补丁与哈希，并通过 `python3 scripts/vendor-1panel.py --verify`。不能为绕过检查而把整个上游目录加入忽略列表。静态分析修正也应保留来源记录。

## 验证

```bash
make check
make security
make integration integration-acme
make test-web
make test-web-dev
```

浏览器脚本创建临时数据、随机访问密钥和私有容器名称，只清理带该临时数据目录 ownership 标签的容器。不要把浏览器回归指向真实数据。测试命令、覆盖和边界见 [验证说明](docs/testing.md)。

按改动运行有关测试；涉及数据库、路径、证书、部署或恢复时必须覆盖失败路径。纯文字与样式调整无需编写复制实现的测试。更新依赖后重新生成许可证清单：`make notices`；完整构建也会生成清单。

## 发布

VERSION 是版本号唯一来源；更新时同步 web/package.json 与 lock 的根版本，由检查脚本验证。构建信息通过 scripts/build.py 注入；`-version` 不依赖密钥或 Docker。发布流程、校验和及源码包见 [发布说明](docs/releasing.md)。
