# 前端开发与构建

构建链路参照固定版本 1Panel 的 `frontend/vite.config.ts`、`tsconfig.json`、`main.ts` 和根目录 Makefile。应用使用 Vue 3、Vue Router、Pinia、Vue I18n、Element Plus 和 Vite；上游源码与本地业务源码在同一个应用中编译。

## 开发

首次运行先执行 `make build`，生成 Go 嵌入所需的静态文件，然后分别启动后端与 Vite：

```bash
make dev CONFIG=/absolute/path/webcp.json
npm run dev --prefix web
```

打开 `http://127.0.0.1:5173`。Vue 单文件组件、SCSS、SVG 和路由组件由 Vite 编译并热更新，`/api` 默认代理到 `http://127.0.0.1:9080`。可通过 `RESTY_DEV_API` 指定另一个后端；代理保留 Origin，兼容 Go 的同源请求检查。

## 生产

```bash
make build
```

顺序为 `npm ci` → `vue-tsc --noEmit` → `vite build --mode production` → Go 编译/嵌入 `web/dist`。`npm run build:pro --prefix web` 也可单独构建前端。

`web/src` 与 `web/panel/src` 全部纳入 TypeScript 检查。路由按需加载，共享唯一的 Vue、Vue Router、Pinia、I18n 和 Element Plus 实例。Element Plus 基础 CSS、深色变量及 1Panel 样式随应用统一加载。CSS 合并为单个文件，避免异步页面样式加载次序改变公共组件。

与上游一致，插件负责 Vue/JSX、Vue/路由 API 自动导入、组件发现、Element Plus 组件解析和 SVG 组件转换。`web/auto-imports.d.ts` 与 `web/src/element-components.d.ts` 是插件生成的实际类型索引，保留在源码中，让首次类型检查可直接执行；Vite 自动更新它们。

## 上游复用边界

- `web/panel/src/layout`、`router-view-cache`、`SubItem`、侧栏 SCSS、渐变 SVG、主题切换和 ComplexTable/FuTable 来自固定提交的 1Panel。
- 一个 Vue Router 管理侧栏、顶部入口、SSL 子页和网站设置查询参数；前进/后退与菜单高亮同步，切换页面后内容区域回到顶部。
- 使用上游的页面淡入和布局过渡。ElMenu 的 `collapse-transition=false` 也保持上游设置；侧栏宽度、主体边距、手机侧滑动画由外层布局负责。
- 小于 1200px 自动折叠，手机使用侧滑导航，选择页面后关闭；本地领域列表通过原版 ComplexTable 的卡片模式适配手机。
- Pinia 同时提供上游组件和本地页面状态，深色模式支持跟随系统。商业授权、多用户账户和节点切换依赖按独立程序范围移除；控制台密钥会话由独立认证模块提供。
- 每个引用源文件的提交、哈希与补丁见 `web/panel/manifest.json`。本地领域适配集中在 `web/src/components/panel`。

## 验证

按照 [测试说明](testing.md) 启动隔离后端，再执行：

```bash
npm run test:e2e:dev --prefix web
npm run test:e2e --prefix web
```

开发模式由 Playwright 启动 Vite 并代理到隔离后端；生产模式直接访问嵌入最新静态文件的隔离 Go 程序。两者均检查浏览器异常，开发模式另外检查 Vue 的组件解析、注入、属性和渲染警告。

框架回归覆盖路由历史、选中状态、页面过渡、侧栏动画、渐变 SVG、系统主题、自动折叠、手机导航与卡片操作。HTTPS 回归还覆盖取消、失败后重试与服务器文件选择器；仅失败重试用例拦截一次保存请求以复现错误，其他操作使用真实 API。

正式构建先运行 Vite 生成可嵌入资源，再收集实际 Go 依赖许可证并同步到 dist/licenses，最后编译 Go。这样干净源码目录无需预先存在 Web 构建目录，也不会把过期许可声明嵌入二进制。版本由根目录 VERSION 注入，发布包需通过源码重新构建验证。
