# SQLite 与 OpenResty 文件职责

## 设计依据

本项目采用控制状态与服务文件分离的方式：SQLite 保存控制器状态和结构化 OpenResty 配置，OpenResty 自己读取并提供 HTML 文件。界面通过文件服务读取、编辑页面，Go 不参与网站错误响应。

参考固定版本 1Panel 的 [数据库初始化](https://github.com/1Panel-dev/1Panel/blob/3adee9445897aa41c0160b4f805a9b1f43f712ac/agent/init/db/db.go) 和 [GetDefaultHtml / UpdateDefaultHtml](https://github.com/1Panel-dev/1Panel/blob/3adee9445897aa41c0160b4f805a9b1f43f712ac/agent/app/service/website.go)：业务元数据放 SQLite，默认 HTML 在 OpenResty root/default 下按文件读写。沿用其页面文件归属和 GUI 组件；存储适配采用 Go database/sql 与无 CGO 的 modernc SQLite 驱动，保持独立二进制的交付方式，不引入面板全局数据库对象。

配置组织参考 [NGINX 官方配置管理](https://docs.nginx.com/nginx/admin-guide/basic-functionality/managing-configuration-files/)；页面提供方式使用 [NGINX 静态文件服务](https://docs.nginx.com/nginx/admin-guide/web-server/serving-static-content/) 的 root、try_files 和 error_page。SQLite 参数依据 [WAL](https://www.sqlite.org/wal.html) 和 [PRAGMA](https://www.sqlite.org/pragma.html) 官方文档。

## 数据归属

| 内容 | 权威存储 | 写入方式 |
| --- | --- | --- |
| 网站、域名、反向代理、重定向、L4、Lua、证书、SSL/mTLS、S3 参数 | data/webcp.db 的独立资源表 | SQLite 事务及 revision 比较交换 |
| OpenResty 性能参数 | openresty_settings | 与配置事务一起提交 |
| 本机容器连接 | runtime_connection | 独立 version，排除在便携备份外 |
| 备份索引和恢复操作 | backup_records / restore_operations | SQLite 事务 |
| 全局默认页面 | data/runtime/root/default/{code}.html | 文件接口、内容 ETag、原子目录交换 |
| 网站独立页面 | data/runtime/root/sites/{site-id}/{code}.html | 同上；不存在时继承全局文件 |
| 网站内容 | data/sites/{site-id}/ | 文件接口或外部文件部署 |
| Nginx、Lua 和部署证书 | data/runtime/generations/{id}/ | 由配置生成的不可变部署版本 |

State 和 Site 类型没有 pages 字段；SQLite 没有页面内容表。网站资源表额外检查 JSON 中不得出现 pages。状态聚合只用于领域校验、API 查询和便携导出，数据库按独立资源记录增量更新，不以完整状态 JSON 作为主存储；恢复操作仅在执行期间保留回滚所需的配置。

证书、SSL 和 mTLS 继续复用既有 1Panel 模型、组件及签发核心。资源表的 document 限于该类型的嵌套配置，不提供任意键值配置接口。

## SQLite 提交与版本

数据库使用版本化 SQL 迁移、STRICT 表、主键、网站与证书及 mTLS 引用的延迟外键、JSON 检查约束、WAL、synchronous=FULL、busy_timeout=5000。写连接上限为 1，事务以 IMMEDIATE 获取写锁；只读连接池上限为 4，使用 query_only 与独立读事务。WAL 读者只看已提交快照。

配置命令以 revision 执行事务内比较交换。元数据和所有相关资源在同一事务提交；失败不留半写入状态。页面文件和普通网站文件编辑不递增配置 revision，也不触发 Nginx 重载。页面的内容 ETag 单独防止界面、其他浏览器和文件系统编辑之间的过期覆盖。

启动在打开数据库和执行迁移之前取得数据目录 flock。数据库文件权限为 0600，数据目录权限为 0700。数据库、WAL、SHM、密钥配置和备份目录均不挂载到 OpenResty。

## 页面读写与服务

默认页面首次初始化为 OpenResty 可直接提供的 HTML 文件。首次导入旧自定义页面优先；初始化标记记录完成状态，后续启动不会用模板覆盖管理员的修改，也不会重新生成主动移除的文件。

编辑界面读取实际文件，返回 pages 与 etag。保存同时携带读取时的配置 revision 和内容 etag；页面内容变化返回 409。文件大小限制为每页 256 KiB，状态码限制为 403、404、495、496、500、502、503、504。不接受符号链接或路径穿越。

同一作用域的页面集合先写入临时目录，逐文件 fsync，再使用 Linux renameat2(RENAME_EXCHANGE) 原子交换目录并同步父目录。保存失败不会逐个覆盖出半套页面。

Nginx 的内部错误 location 使用固定的 /srv/resty/root 路径和 try_files，先找网站页面，再找全局页面；495/496 保持 HTTP 403 的既有语义。HTML 不再复制进 generation。页面缺失时保留原错误状态码，使用 Nginx 自带错误响应。修改和移除覆盖文件即时生效；直接请求内部错误 URI 仍被拒绝。

托管容器与已绑定 runtime 的容器直接读取挂载文件。无绑定挂载的已有容器在应用时同步页面文件，界面保存时也同步受管 HTML 的新增、修改和删除，移除独立页面后立即恢复继承；停止或尚未接管的容器在下一次应用时接收文件。同步失败明确报告“文件已保存、同步失败”，可重试保存或应用。该模式的外部修改应在宿主机权威目录进行，容器副本会在下次同步时更新。

## 旧数据迁移

首次打开数据库时：

1. 验证旧 state.json，保留 revision、网站 ID、证书材料和所有业务配置。
2. 将全局及网站 pages 分离成 OpenResty 文件；不覆盖已存在的文件修改。
3. 使用当前 schema=2 将配置写入 SQLite 事务。
4. 原 state.json 改名为 state.json.migrated，作为一次性原始备份保留，后续运行不读取它。
5. 迁移 runtime/connection.json 和 backups/*.json 到对应 SQLite 表，原文件保留 .migrated 副本。

旧配置解析、证书验证或数据库迁移失败时拒绝启动，不回退为空数据库，也不继续双写 JSON。SQL schema 由 PRAGMA user_version 管理，目前为 1；它与便携导出 schema=2 是两个独立版本。

.migrated 文件是历史备份，可能含旧 HTML、私钥和凭据，保持私有权限。它们不是新的状态来源，也不包含在便携备份中。

## 备份恢复

新 tar.gz 归档包含：

- state.json：从 SQLite 一致性读取的 schema=2 便携配置，不含 HTML。
- sites/{site-id}/：网站文件。
- openresty/root/default/ 与 openresty/root/sites/{site-id}/：独立页面文件。

归档不复制运行中的 SQLite/WAL 文件，因此不依赖对 WAL 数据库文件的非一致性文件拷贝。S3 继续使用相同归档与 SHA-256。

恢复先验证摘要、文件类型、路径、大小和配置，停止 OpenResty，再将旧配置写入 SQLite 恢复日志，替换网站目录和页面目录，最后提交新配置并删除日志。提交前中断会在下次启动恢复旧目录及旧配置；恢复后保持停止，等待检查和应用。

旧 schema=1 归档仍可恢复，其中 HTML 在解包时转成页面文件，证书和配置进入 SQLite。当前归档中的页面删除也会保留，不会自动填回模板。

## 验证范围与边界

测试覆盖 JSON 迁移及原件保留、SQLite 文件格式和 PRAGMA、重启持久化、跨连接 CAS、事务失败回滚、WAL 隔离、页面文件 ETag、外部修改、删除后的重启、归档恢复和中断恢复。实际 OpenResty 测试检查 mTLS 证书错误页面、网站覆盖，以及无挂载已有容器的即时页面同步；页面修改不改变配置 revision 或 generation。

单机 SQLite 不适用于多个宿主机共享网络文件系统。本程序保留单进程单写入口；耗时部署和备份仍由 Manager 串行协调。文件 ETag 能检测保存前已经发生的外部修改，不对不遵守本程序锁的外部进程提供跨进程写事务；一致性备份期间需暂停外部文件部署。

发布检查补充了压缩归档完整尾部/校验和、额外 gzip 成员、隐藏 tar 内容、空页面导入和无效恢复日志的失败验证。外部空页面可以在界面修复，已初始化的页面文件异常不会阻止控制器数据库启动。
