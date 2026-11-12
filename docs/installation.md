# WebCP 安装与升级

WebCP 是独立的 OpenResty 网站控制程序，Web 已内嵌，无须安装 Node.js 或数据库服务。正式包支持 Linux amd64 和 arm64，需要 Docker CLI、运行中的 Docker Engine，以及访问 Docker 的权限。

## 安装

1. 从同一次发布取得架构对应的 tar.gz、SHA256SUMS 和同版本源码归档。使用 `sha256sum -c SHA256SUMS --ignore-missing` 核对已下载文件。
2. 解压二进制归档，进入目录，执行 `./webcp -version` 查看版本、Go 工具链和来源提交。
3. 生成本机配置和随机访问密钥：

```bash
umask 077
python3 - <<'PY'
import json, os, secrets
with os.fdopen(os.open('webcp.json', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as output:
    json.dump({'accessKey':secrets.token_urlsafe(32),'listen':'127.0.0.1:9080','data':'./data'},output,indent=2)
    output.write('\n')
PY
./webcp -config ./webcp.json
```

打开 http://127.0.0.1:9080，用 webcp.json 的 accessKey 登录。也支持 WEBCP_ACCESS_KEY 或 WEBCP_ACCESS_KEY_FILE 注入，两个环境变量不能同时设置；未配置密钥拒绝启动。密钥需要 16–4096 字节。不要把真实配置放入源码仓库或发布包。

首次启动不创建或接管任何容器。在 OpenResty 的“容器连接”选择托管容器、自定义镜像或已有容器；保存仅保存连接，点击“校验并应用”才部署。已有容器首次接管前备份原主配置，原有网站不会自动导入。容器端口映射由 Docker 决定，接管不重建已有容器。

管理监听默认在回环地址。远程测试可使用 `ssh -L 9080:127.0.0.1:9080 user@server`。经 HTTPS 反向代理时，将配置 secureCookie 设为 true，并保留原始 Host：`proxy_set_header Host $http_host;`。反向代理的请求体上限应至少为 10 MiB，并按备份操作需要设置读取超时。退出或重启服务后需重新登录。

## 数据目录

- webcp.db、WAL、SHM：SQLite 控制状态、配置、证书和凭据，保持私有权限。
- sites/：网站内容。
- runtime/root/default/：OpenResty 默认 HTML 页面；runtime/root/sites/：网站独立页面。
- runtime/generations/、current：已生成的 Nginx、Lua、部署证书和当前版本链接。
- backups/：便携备份；ssl/logs/：证书任务日志；cache/：可重建缓存。

默认页面直接由 OpenResty 提供；控制器只读写文件，不在 SQLite 保存页面 HTML。修改页面即时生效，不触发重载；页面编辑使用独立内容版本防止过期覆盖。

启动参数中 data 应在正式部署时使用绝对路径。GUI 中已保存的容器连接优先于启动连接默认值。SQLite 数据目录放在本地文件系统，不供多个宿主机并发共享。

## 升级与回退

1. 停止 WebCP，等待进程完全退出；停止控制器不会停止已有 OpenResty。
2. 备份启动配置和整个数据目录。SQLite 运行期间不能只复制单个 webcp.db 作为完整备份；停进程后复制完整目录，或使用 GUI 创建便携备份。
3. 保留旧可执行文件，再替换新二进制，以原配置启动。
4. 检查版本、网站、证书、连接和日志。存在旧部署路径时，核对草稿后应用一次配置，使 OpenResty 使用新的页面文件路径。

旧 JSON 首次启动自动迁移到 SQLite，并将内联页面分离为 HTML 文件；原件保留为 .migrated。新版本导出格式为 schema=2，兼容 schema=1 便携备份恢复。恢复便携备份会停止 OpenResty，并保持停止，需核对后应用。

回退必须停控制器并成套恢复旧二进制、旧配置和升级前整个数据目录；不要仅恢复旧二进制，也不要让旧程序读取迁移后的目录。若要回退已经应用的新部署，还需检查旧 generation 和容器端口配置，再应用恢复的配置。

## 服务管理与限制

源码包包含可调整的 docs/deploy/webcp.service 示例，使用 /usr/local/bin/webcp、/etc/webcp/webcp.json 和 /var/lib/webcp。该示例限制写入 /var/lib/webcp；证书推送或后置脚本需要访问其他目录时，应相应调整服务的 ReadWritePaths。服务进程必须能够连接 Docker，不能直接套用示例到不同路径。

本程序面向单机可信管理员，不提供多租户隔离。Lua 和证书后置脚本按管理员代码执行。HTTP/3 已验证配置、端口发布和 Alt-Svc，未宣称完成端到端 QUIC 客户端覆盖。部署及恢复支持进程中断回滚，不承诺 Docker 与文件系统之间的断电原子事务。

许可证与来源见随包 LICENSE、NOTICE、DEPENDENCIES.json 及 THIRD_PARTY_NOTICES 文件。源码和构建说明在同版本 source.tar.gz 中；校验和仅验证文件一致性，不等同于签名或构建来源证明。
