# 1Panel GUI 复用与操作约定

前端基于 1Panel `dev-v2` 固定提交 `3adee9445897aa41c0160b4f805a9b1f43f712ac` 改造，直接使用其主题 SCSS、布局和界面组件，以及同版本 Element Plus 2.14.7。

## 直接复用

布局、菜单子项、路由过渡、主题机制和完整表格来自 `web/panel/src`，来源与补丁见 [源码清单](../web/panel/manifest.json)。

`web/src/vendor/1panel` 保留 GPL 标记及来源。逐文件路径、Git blob SHA 和修改原因见 [manifest.json](../web/src/vendor/1panel/manifest.json)。

- 主题：element.scss、element-dark.scss、common.scss、reset.scss、fu.scss，保留 `#005eeb` 主色、Element Plus 样式覆盖及深色配色。
- 导航：侧边菜单 SCSS、菜单背景 SVG，侧栏宽 180px、折叠后 75px。
- 布局：LayoutContent、form-button 的 app/搜索/工具栏/main 插槽；CardWithHeader、MainDiv。
- 容器：DrawerPro、DialogPro，保留右侧抽屉、返回/资源标题、全屏和固定页脚；手机抽屉全宽。
- 表格：TableSearch、TableRefresh、TablePagination、TableOperationActions、Status。保留回车/失焦提交搜索、清空重查、分页、行内操作及“更多”菜单。
- 确认：ConfirmDialog 用于恢复，输入“恢复”后才能提交。
- 编辑器：CodemirrorPro 的 CodeMirror 6 + oneDark 和双向 model API。增加 HTML/Lua 和无障碍标签，修复生命周期以避免重复初始化和遗留实例。

## 本地适配

`web/src/components/panel` 负责接口与路由适配。

| 组件 | 来源与适配边界 |
| --- | --- |
| RouterButton | 薄适配至原版 RouterButton；网站与 mTLS 共用路由选中机制，嵌套路由优先匹配最长路径，移除授权、订阅、RBAC、节点代码 |
| BackButton | 保留 el-page-header，使用统一 Vue Router 跳转 |
| ComplexTable | 直接使用上游 ComplexTable/FuTable；本地适配仅负责领域 API 的分页数据和手机卡片模式选择 |
| DeleteDialog | 保留警告、对象名称与取消/确认流程，调用者管理 Promise 和业务操作 |
| PanelDrawer | 统一错误显示、忙碌状态和页脚提交 |
| WebsiteFields / WebsiteFiles / ErrorPageEditor | 本项目领域表单和文件/页面编辑，使用同一组件体系 |

源码本地保留，构建和运行不下载上游文件。升级时检查固定版本和 manifest，不直接覆盖 vendor 目录。页面通过 Vue Router 异步加载；Element Plus 插件、基础样式和上游主题统一初始化。具体流程见 [前端工作流](frontend-workflow.md)。

## 操作约定

1. 网站列表采用左侧创建/高级设置，右侧筛选/搜索/刷新，底部分页。行内操作为配置、备份和更多；状态按钮可点击操作。
2. 创建四层代理、网站、证书、Lua 资源和 S3 配置使用右侧抽屉，字段标签在上，取消与确认/保存在固定页脚。
3. 网站“配置”进入独立设置页，左侧为域名设置、网站目录、默认文档、反向代理、跨域设置、HTTPS、真实 IP、重定向、错误页面、Lua 扩展；地址保留网站和分区，可刷新和返回。
4. “高级设置”打开默认页面抽屉，选择错误页后用同一暗色代码编辑器修改 HTML。
5. OpenResty 运行时，网站/页面/Lua/客户端吊销保存后自动校验并重载。停止时保存草稿，再从状态条启动。S3 配置保存后立即用于备份。
6. 保存成功而部署失败时，明确提示“已保存，但应用失败”，保留草稿和旧部署。用户修正后可重新应用。
7. 删除、停用在界面内确认，恢复使用上游输入确认组件；不使用浏览器原生 confirm 弹窗。
8. 保留菜单折叠、深浅主题和手机菜单；不引入多用户、RBAC、商业授权、订阅或节点管理依赖；控制台以配置注入的密钥登录。

PHP、伪静态、负载均衡、流量限制、运行环境和密码访问等已经排除的功能，不因 GUI 复用重新引入。

## 四层代理

「网站 → 四层代理」沿用 1Panel stream 的协议、监听端口、目标和保存流程，使用本项目已复用的 LayoutContent、TableSearch、TableRefresh、TableOperationActions、Status、DrawerPro 和 DeleteDialog。新增宿主监听地址及高级超时字段，支持筛选、名称/地址搜索、分页、启停、编辑和确认删除。每条规则只允许单个目标，继续遵守不引入负载均衡的范围约定。端口发布变化重建容器的中断提示常驻列表上方。

网站各模块独立保存、规则抽屉、确认流程及与上游行为的对应见 [网站设置说明](website-settings.md)。

## SSL 原版页面与独立 mTLS

SSL 使用固定提交的完整页面源码及实际 ComplexTable/FuTable 依赖，不再使用本地混合证书页面。网站 HTTPS 使用原版组件。mTLS 独立入口复用原版 CA 页、抽屉和确认框；专有的客户端/绑定逻辑由独立模块实现。具体清单、逐文件补丁及允许的适配见 [证书模块说明](certificates.md)。

## OpenResty 性能调节

“OpenResty → 性能调节”直接使用固定提交 `website/website/nginx/performance/index.vue` 的表单、字段、帮助文字、布局及单位处理。支持域名哈希桶、请求头缓冲区、请求体大小、keepalive、gzip 开关/最小长度/压缩级别、三类代理超时，共 10 项。表单读取实际生成配置，保留字节/K/M/G 单位和复合时间表达式。保存使用读取表单时的配置版本；冲突时点击“刷新内容”重新加载。加载期间禁止编辑，避免接口返回值覆盖输入。

配置保存在 `state.openresty.http`，纳入本地/S3 整体备份；旧配置缺少该字段时沿用原有有效默认值。运行中保存后校验并重载，失败保留草稿与旧部署；停止时只保存草稿。HTTP 全局参数遵循 Nginx 继承机制，网站反向代理中显式设置的超时仍优先。独立程序没有动态二进制模块构建，因此原版的 Brotli 模块入口不显示。
