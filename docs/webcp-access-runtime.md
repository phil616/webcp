# WebCP 访问与容器配置

WebCP 表示 Web 控制面板。界面仅使用文字名称，不提供项目 LOGO。

## 密钥注入

启动必须提供 16–4096 字节的访问密钥，支持三种来源：

- `-config /path/to/webcp.json` 的 `accessKey` 字段。
- `WEBCP_ACCESS_KEY` 环境变量，覆盖 JSON 中的密钥。
- `WEBCP_ACCESS_KEY_FILE` 指定密钥文件，适合 Docker secrets；移除文件末尾换行。

两个密钥环境变量不能同时设置。访问密钥不会写入网站状态或 S3 备份，不会通过 API 返回，也不放在 URL 或浏览器 localStorage。示例结构见根目录 `webcp.example.json`；其中密钥为空，必须自行填写。建议用随机生成的密钥并将配置文件权限设为 0600。

JSON 支持 `listen`、`data`、`container`、`image`、`httpBind`、`httpsBind`。命令行同名参数覆盖启动 JSON。GUI 保存的容器连接存放在 `data/webcp.db` 的 `runtime_connection` 表，后续启动沿用该连接，启动参数作为初始连接默认值。机器本地连接和访问密钥都不包含在网站备份中。

打开管理端口后先输入密钥。服务端使用随机会话和 HttpOnly、SameSite=Strict Cookie；会话有效期 12 小时。退出会撤销会话，服务重启也会使会话失效。密钥更新后重启服务生效。HTTPS 请求自动使用 Secure Cookie；通过 HTTPS 反向代理访问时，在 JSON 中设置 `secureCookie: true`。

## 选择 OpenResty

入口为 **OpenResty → 容器连接**。

### 创建或使用托管容器

填写容器名称、镜像、HTTP/HTTPS 发布地址，点击“保存连接”。镜像可使用自建 OpenResty。Docker 中没有 OpenResty 时，“校验并应用”会按所填镜像创建容器。保存连接本身不拉取镜像、不创建或停止容器。

托管模式继续使用所有权标签，防止同名容器被意外覆盖。已有同名且非此数据目录的容器应选择“连接已有容器”。

### 接管已有容器

列表显示全部 Docker 容器，支持搜索和直接输入容器名，不依赖镜像名称识别。首次检测需要容器运行。WebCP 会检测 OpenResty、LuaJIT、主配置、工作进程和 MIME 配置；非标准路径可在“高级连接设置”指定。

接管方式按用户选择：**WebCP 备份原主配置，然后接管主配置**。检测和保存连接不改动主配置；首次“校验并应用”才执行备份与部署。原有网站不会自动导入，需要先在 WebCP 创建相应网站。

原主配置备份位于：

```text
<data>/runtime/external/<container-id>/original-nginx.conf
```

备份权限为 0600，后续部署不覆盖该原始备份。每次应用另保存当次配置快照；失败或中断会通过日志恢复上次配置。接管不重建或重命名原容器，以容器 ID 固定目标，原 Docker 端口映射和网络保留。WebCP 检查所需映射；新增监听若缺少映射，需要在 Docker 中补充后再应用。

若容器已挂载 WebCP 的 runtime、sites、cache 目录，直接使用挂载。普通容器没有这些挂载时，WebCP 自动同步生成配置、证书、Lua 和网站文件到容器内的保留目录，文件编辑及 HTTP-01 验证文件也会同步。复制模式使用独立的网站版本目录，保留符号链接防护。大量文件建议使用绑定挂载以减少部署复制成本。

主配置替换为指向 WebCP 当前版本的入口。原配置中引用的其他文件不会被修改；需要人工恢复时，可将 `original-nginx.conf` 复制回检测到的容器主配置路径，验证配置后重载。容器连接页显示本次接管目标和检测路径。
