# HTTP API

前缀 `/api`，JSON 响应。管理 API 均要求有效登录会话；先 `POST /api/auth/login` 提交 `{key}`，随后携带服务端发出的 HttpOnly Cookie。未登录返回 401。仅允许同源浏览器写入请求。跨域部署前端时请用同一反向代理提供前端和 `/api`；不要放宽跨站保护。

配置修改、文件写入、部署、恢复必须带 `If-Match: <revision>`，revision 从 overview.state.revision 获得。JSON 请求同时发送 `Content-Type: application/json`。原生 JSON DTO 严格拒绝未知字段，错误为 `{"error":"..."}`，过期 revision 返回 409，不存在返回 404，其余无效命令返回 400。

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | /version | 构建版本、提交、时间、Go 版本与平台，要求登录 |
| GET | /overview | 草稿（私钥及 S3 Secret Key 脱敏）与运行状态 |
| POST | /sites | 新建/更新网站，空 id 为新建 |
| DELETE | /sites/{id} | 删除网站草稿，保留磁盘文件 |
| GET | /sites/{id}/config | 原子读取网站配置草稿 `{content,revision}` |
| PUT | /sites/{id}/config | `{content}`，完整解析并映射到 GUI；If-Match 使用读取时的 revision |
| PUT | /sites/{id}/advanced | `{headers,security}`，结构化响应头及目录保护 |
| GET | /sites/{id}/files | 列举网站文件 |
| GET | /sites/{id}/file?path=index.html | 读取 UTF-8 文本文件 |
| PUT | /sites/{id}/file | 原子保存 `{path,content}` |
| POST | /sites/{id}/upload?path=logo.png | 原始文件上传，Content-Type 为 application/octet-stream |
| DELETE | /sites/{id}/file?path=index.html | 删除普通文件 |
| POST | /streams | 新建/更新单目标四层代理，空 id 为新建 |
| DELETE | /streams/{id} | 删除四层代理草稿 |
| POST | /lua | 新建/更新 `{id,name,kind,description,code}` |
| DELETE | /lua/{id} | 删除未被引用的资源 |
| GET | /pages | 从 OpenResty 全局页面文件读取 `{pages,etag}` |
| PUT | /pages | 保存 `{pages:{"404":"<h1>404</h1>"},etag:"读取时的版本"}`，不递增配置 revision |
| GET | /sites/{id}/pages | 读取网站独立页面文件 `{pages,etag}` |
| PUT | /sites/{id}/pages | 保存 `{pages,etag}`，空 pages 表示继承全局文件 |
| GET | /config | `{config}`，当前草稿的 Nginx 配置预览 |
| POST | /runtime/apply | 校验并部署草稿；端口发布变化时重建容器并支持失败回滚 |
| POST | /runtime/stop | 停止托管容器，不要求 If-Match |
| GET | /runtime/logs | `{logs}`，最近 200 行 |
| PUT | /s3 | `{endpoint,region,bucket,prefix,accessKey,secretKey,pathStyle}` |
| POST | /s3/check | 检查已保存 S3 的写入、回读、列举和删除权限 |
| GET | /backups?remote=false | 本地备份列表，remote=true 列举配置前缀下 S3 备份 |
| POST | /backups | `{remote:false}` 创建本地完整备份，true 同时上传 S3 |
| GET | /backups/{id}/download | 下载本地 tar.gz 备份 |
| POST | /backups/{id}/restore | `{remote:false}` 恢复本地备份，true 从 S3 恢复 |

网站保存示例：

```json
{
  "id": "",
  "name": "Docs",
  "domains": ["docs.example.com"],
  "kind": "static",
  "enabled": true,
  "upstream": "",
  "tlsId": "",
  "luaBindings": {}
}
```

overview 不返回页面 HTML；State 与 Site 不接受 pages 字段。页面接口同时校验 If-Match 配置版本和内容 etag，内容冲突返回 409。控制器状态由 SQLite 保存，页面直接读写 OpenResty 文件，见 [持久化说明](persistence.md)。

Lua 模块名必须为 `[A-Za-z_][A-Za-z0-9_]{0,63}`，全局唯一。代码块绑定格式为 `{"access":"<block-id>"}`。引用资源不能先删除。

S3 endpoint 必须包含 http(s) 协议，不支持路径。Secret Key 留空保留已保存的密钥。S3 设置保存后即可用于备份，无需重载 OpenResty；它仍计入配置 revision，以便备份与恢复完整版本。

文件修改即时生效，不递增配置 revision。编辑与上传单文件上限 4 MiB，JSON 请求上限 10 MiB。配置快照限 16 MiB。备份展开总量 2 GiB、单文件 128 MiB、条目 100000；支持的备份必须由本程序生成并包含 SHA-256 元数据。

## 四层代理

规则位于 `overview.state.streams`，旧 schema-1 状态缺少此字段时加载为空数组。保存示例：

```json
{
  "id": "",
  "name": "数据库转发",
  "protocol": "tcp",
  "listenAddress": "127.0.0.1",
  "listenPort": 15432,
  "targetHost": "host.docker.internal",
  "targetPort": 5432,
  "connectTimeout": 10,
  "proxyTimeout": 600,
  "udpResponses": 1,
  "proxyProtocol": false,
  "enabled": true
}
```

`protocol` 为 tcp/udp。监听地址为宿主机 IP，默认 0.0.0.0；目标仅支持单个域名或 IP，不含 URL 协议、方括号或端口。端口范围 1–65535。连接超时 1–3600 秒、空闲超时 1–86400 秒，省略或 0 时分别使用 10、600 秒。udpResponses 范围 0–1000，0 表示不等待响应，GUI 默认 1。proxyProtocol 仅支持 TCP。createdAt 由服务端维护。

启用规则按协议检查监听冲突，TCP/UDP 可共用宿主机端口；TCP 不能与 HTTP/HTTPS 发布地址重叠。GUI 停用是保存同一 ID 的 enabled=false。保存只修改草稿，外部 API 客户端需调用 `/runtime/apply` 部署。GUI 在运行时自动调用该接口；失败保留草稿并报告错误。

`overview.runtime.publishedPorts` 返回实际容器发布记录 `{hostAddress,hostPort,containerPort,protocol}`，未创建容器时为空数组。containerPort 为实现细节，不应写入规则或作为稳定标识。应用版本以 appliedRevision 为准。完整部署行为见 [四层代理说明](l4-proxy.md)。

## 网站分区接口

以下路径均以 `/api/sites/{id}` 开头。写配置需要 If-Match，保存草稿后通过 `/runtime/apply` 应用；GUI 在运行时自动应用。字段未知或跨分区混用时拒绝请求。

| 方法 | 路径后缀 | 请求 / 响应 |
| --- | --- | --- |
| PUT | /name | `{name}` |
| PUT | /domains | `{bindings:[{domain,port,ssl}]}`，替换完整绑定列表 |
| GET | /directories | 可选运行目录数组，例如 `["/","/public"]` |
| PUT | /directory | `{runDirectory:"/public"}` |
| PUT | /index | `{index:["home.html","index.html"]}` |
| PUT | /https | `{tlsId,https:{httpConfig,protocols,ciphers,hsts,hstsIncludeSubDomains,http2,http3}}` |
| PUT | /cors | CORSConfig |
| PUT | /real-ip | `{enabled,from:["192.0.2.0/24"],header:"X-Forwarded-For"}` |
| POST | /proxies | ProxyRule，空 id 创建，已有 id 更新 |
| DELETE | /proxies/{rule} | 删除指定规则 |
| GET | /proxies/{rule}/config | `{config}`，生成的只读片段 |
| PUT | /proxy-cache | `{memoryMb:5,maxSizeMb:1024,inactiveMinutes:60}` |
| POST | /proxy-cache/clear | 清理当前网站缓存，操作命令，不要求 If-Match |
| POST | /redirects | RedirectRule，空 id 创建，已有 id 更新 |
| DELETE | /redirects/{rule} | 删除指定规则 |
| GET | /redirects/{rule}/config | `{config}`，生成的只读片段 |
| PUT | /extensions | `{luaBindings}`；页面文件使用独立 /pages 接口 |

HTTPS 的 httpConfig 为 `HTTPToHTTPS`、`HTTPAlso`、`HTTPSOnly`；protocols 为原版 TLSv1/TLSv1.1/TLSv1.2/TLSv1.3 数组，默认 1.2/1.3。空 tlsId 关闭 HTTPS；仍存在启用的 mTLS 绑定时禁止关闭。mTLS 不允许 HTTPAlso。证书及 CA 使用下方 SSL/mTLS 独立接口。

CORSConfig 为 `{enabled,allowOrigins,allowMethods,allowHeaders,allowCredentials,preflight}`；allowOrigins 支持 *、以换行/逗号分隔的明确 http(s) origin 或 `$http_origin`。凭据模式不能使用 *。allowMethods 和 allowHeaders 为逗号分隔的 token。

ProxyRule 示例：

```json
{
  "id": "",
  "name": "api",
  "enabled": true,
  "modifier": "",
  "match": "/api/",
  "proxyPass": "http://host.docker.internal:9000/backend/",
  "proxyHost": "$host",
  "sni": false,
  "sslName": "",
  "sslVerify": false,
  "connectTimeout": 60,
  "readTimeout": 60,
  "sendTimeout": 60,
  "cache": false,
  "serverCacheTime": 10,
  "serverCacheUnit": "m",
  "cacheTime": 0,
  "cacheUnit": "h",
  "replaces": [],
  "cors": {"enabled": false}
}
```

modifier 为空、=、^~、~、~*。正则目标不能含 URI。每条规则只支持一个 HTTP(S) 目标。三个超时为 1–3600 秒，省略或 0 时取 60。缓存时间单位 s/m/h/d，正数范围 1–65535；cacheTime=0 不修改浏览器缓存，-1 禁用，正数设置过期。replaces 为 `{from,to}` 数组。名称创建后不可更改。规则级 CORS 启用时覆盖网站默认。

RedirectRule 示例：

```json
{
  "id": "",
  "name": "old_path",
  "enabled": true,
  "type": "path",
  "domains": [],
  "path": "/old/",
  "redirect": 302,
  "target": "https://destination.example/new/",
  "keepPath": true,
  "redirectRoot": false
}
```

type 为 domain/path/404，redirect 为 301/302。域名规则填写已绑定来源 domains；路径规则填写 path；404 的 redirectRoot=true 时跳转首页，可省略 target。名称与类型创建后不能更改。启停规则是更新同一 ID 的 enabled。路径、查询保留与配置优先级见 [网站设置说明](website-settings.md)。

POST /sites 仍可新建或整体更新。bindings 缺失时由旧 domains 推导 80 端口绑定；提供 bindings 时由它生成去重的 domains。proxies 缺失时，旧 kind=proxy + upstream 迁移为 default 规则；明确 proxies=[] 则不生成。新客户端应使用独立分区接口更新，避免整体替换其他分区。

## 原版 SSL 适配与独立 mTLS

原版组件通过 `/api/panel` 前缀访问原版 DTO。例如 POST `/api/panel/websites/ssl/search`、`/websites/ssl`、`/websites/ssl/upload`、`/websites/ssl/obtain`、`/websites/ssl/del`，以及 `/websites/acme`、`/websites/dns`、`/websites/ca` 的创建/搜索/更新/下载/删除接口。详情为 GET `/websites/ssl/{numericId}`、`/websites/ca/{numericId}`。原版响应 data 由前端传输桥包装；HTTP 响应体直接返回业务值。适配 DTO 允许原版完整行对象中的额外字段，由服务选择有效字段；提交递增统一 revision，不假造成功响应。

网站 HTTPS：GET/POST `/api/panel/websites/{panelId}/https`，panelId 是网站的数值投影 ID；原生网站 API 继续使用原 UUID。POST 字段为原版 enable/type/websiteSSLId/httpConfig/SSLProtocol/algorithm/hsts/hstsIncludeSubDomains/http3，手动导入也保持原版字段。该接口不接受客户端信任 CA。

mTLS CA 页面复用相同 DTO，通过 `/api/panel/mtls/ca` 的创建、search、import、upload、download、del、GET 数值详情访问独立存储。CA 下载 ZIP 为 ca.crt/ca.key。mTLS 其他接口：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| POST | /api/mtls/issue | `{name,kind:"client",caId,days,keyType}`，caId 只能属于 mTLS；原生 If-Match |
| POST | /api/mtls/clients/{id}/revoke | 吊销客户端，保留记录；原生 If-Match |
| PUT | /api/mtls/bindings | `{siteId,caId,enabled}`；空 caId 解除绑定；原生 If-Match |
| DELETE | /api/mtls/authorities/{id} | 退役无绑定且全部客户端已吊销的 CA；原生 If-Match |
| GET | /api/mtls/certificates/{id}/download | 证书 PEM；key=true 导出私钥，CA 可用 crl=true 导出 CRL |

旧 `/api/certificates/*` 混合接口已移除。旧 clientCaId 只用于读取历史配置，不再接受网站写入使用它。

## 控制台访问与容器连接

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | /auth/status | 当前会话状态，无需登录 |
| POST | /auth/login | 以 `{key}` 登录，无需 If-Match |
| POST | /auth/logout | 注销当前会话 |
| GET | /runtime/connection | 当前容器连接配置及其独立 version |
| PUT | /runtime/connection | 保存连接，If-Match 使用连接 version |
| GET | /runtime/containers | 全部 Docker 容器的 ID、名称、镜像和状态 |
| POST | /runtime/connection/probe | 检测 OpenResty、主配置、执行用户和数据接入方式 |

连接配置为机器本地设置，不随网站备份迁移；访问密钥不会通过配置 API 返回。
