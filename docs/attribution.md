# 来源与许可证

本项目按用户要求研究了 [1Panel-dev/1Panel](https://github.com/1Panel-dev/1Panel) 的 dev-v2 分支。2026-11-11 查看了以下文件：

| 来源 | 参考用途 | 读取到的 Git blob SHA |
| --- | --- | --- |
| agent/app/service/website.go | 网站能力划分、网站配置交互范围、UpdateStream 应用流程 | bb011b619ea60d274e550ae31132b085642b2b45 |
| agent/utils/nginx/components/server.go | Nginx server 结构与指令组织 | 5548a8ada52a1d846ad640e4de004c3124c73f6b |
| agent/app/service/website_ssl.go | 网站证书能力划分 | 29b0dc838c7d02804cb6b7a8d396788d55be4523 |
| frontend/src/views/website/website/index.vue | 网站表格、状态与操作布局 | 7da02d561fbe6634c0ef95fb547f45dd16025610 |
| frontend/src/views/website/website/config/basic/stream/index.vue | 四层转发协议、监听端口、目标表单与保存流程参考 | 85e9a95baa60ffcfbd0d47b4154ba00249fef0c2 |
| LICENSE | 上游 GPL-3.0 许可证 | 见上游仓库 |

后端直接复用 SSL ACME/DNS 核心和 CA 密钥/导入函数，业务持久化和运行控制独立实现；前端直接复用并适配 1Panel 主题、布局、抽屉、对话框、搜索、分页、行操作、状态和 CodeMirror 组件。固定提交为 `3adee9445897aa41c0160b4f805a9b1f43f712ac`，修改日期 2026-11-11 至 2026-11-12。完整来源、blob SHA 和修改原因见 [完整复用清单](../web/panel/manifest.json)，适配边界见 [GUI 说明](gui-1panel.md)。

没有引入上游 agent/core、面板全局服务注册、RBAC、授权、应用商店、PHP 或运行环境依赖。复用文件保留 GPL 标记和上游贡献者版权说明。程序采用 GPL-3.0，完整文本见根目录 LICENSE；1Panel 名称和标识属于其权利人，本项目不是官方 1Panel 产品。

默认配置使用 1Panel 发布的 OpenResty 镜像，镜像作为独立容器由 Docker 运行。使用者可通过 -image 选择兼容镜像；需要提供 OpenResty、LuaJIT、标准 mime.types 和系统 CA bundle 的标准路径，以及 stream 模块（支持 proxy_half_close，Nginx 1.21.4+）。

主要直接依赖：

- Vue 3：MIT；Vite：MIT。
- Element Plus 2.14.7、@element-plus/icons-vue：MIT。
- CodeMirror 6 及语言包、vue-i18n、VueUse：MIT。
- 1Panel 前端源码与样式：GPL-3.0，逐文件固定来源。
- MinIO Go SDK：Apache-2.0。
- Playwright：Apache-2.0，仅测试依赖。

依赖的完整版本由 go.sum 和 web/package-lock.json 锁定，原始许可证随相应包分发。

实现参考文档：

- [OpenResty Docker](https://github.com/openresty/docker-openresty)
- [Lua Nginx Module](https://github.com/openresty/lua-nginx-module)
- [Nginx stream proxy 模块](https://nginx.org/en/docs/stream/ngx_stream_proxy_module.html)
- [Nginx stream core 模块](https://nginx.org/en/docs/stream/ngx_stream_core_module.html)
- [Nginx SSL 模块](https://nginx.org/en/docs/http/ngx_http_ssl_module.html)
- [MinIO Go SDK](https://github.com/minio/minio-go)

本次网站设置模块的固定版本行为参考清单见 [website-upstream.md](website-upstream.md)，实现的字段及交互对应见 [网站设置说明](website-settings.md)。站点目录、默认文档、代理、CORS、HTTPS、真实 IP 和重定向表单按上游流程改造，继续通过已复用的通用组件呈现。

SSL 页面、依赖组件和 Go 核心的完整固定来源、哈希与补丁见 [证书复用清单](../web/panel/manifest.json)，边界与修改原因见 [证书模块说明](certificates.md)。

发布检查还对固定上游适配器移除了 Nginx lexer 的重复 trim 字符及 SSL 的冗余类型断言；行为保持一致，修改已记录到 web/panel/patches 和 manifest 的适配后哈希。完整依赖版本与许可原文通过 scripts/dependency-notices.py 生成，见 [许可证说明](licenses/README.md)。
