# 正式版本检查与打包

## 版本与范围

根目录 VERSION 是发布版本来源，必须与 web/package.json 和 lock 的根版本一致。当前 1.0.0 为待发布候选；本地打包和 Actions artifact 上传不等于已经创建 GitHub Release。

支持 Linux amd64 与 arm64。Go 最低版本为 go.mod 中的 1.26.11，Node.js 22.12+，CI 使用 24。更新安全补丁时以 [Go 官方工具链列表](https://go.dev/dl/) 和 [govulncheck](https://go.dev/doc/security/vuln/) 为准，并保留锁文件。

## 检查门槛

```bash
make check build
make security
make integration integration-acme
python3 scripts/test-web.py
python3 scripts/test-web.py --dev
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
```

这些命令检查本地文档链接、统一版本、源码格式、1Panel 的 148 份固定来源、完整前端类型检查、生产构建、Go race/vet、依赖漏洞、实际容器/S3/ACME、SQLite 恢复，以及生产和开发模式的浏览器交互。

`make check` 不自动安装安全扫描或浏览器；安全工具通过固定版本 go run 调用，浏览器首次需要在 web 目录执行 `npx playwright install chromium`。正式检查必须使用干净安装的依赖，不能把一次缓存命中当成验证结果。

## 本地构建包

```bash
# 已有 SOURCE_DATE_EPOCH 时复用；在 Git 仓库可使用目标提交的提交时间
export SOURCE_DATE_EPOCH="$(git log -1 --format=%ct)"
make release
cd dist/releases
sha256sum -c SHA256SUMS
```

非 Git 工作目录可不设置 SOURCE_DATE_EPOCH，此时包时间固定为 Unix epoch，构建时间显示 unknown。commit 无法确认时显示 unknown，不虚构源码身份；这类包只作为本地候选供审阅。对外发布应在实际 Git 仓库中，从经过验证的目标提交构建，检查 VERSION 与 vVERSION 标签一致。

可执行 `python3 scripts/verify-release.py dist/releases --source-rebuild` 核对归档边界、版本、静态架构、摘要，并在临时目录从对应源码重新构建，比较原生可执行文件字节。源码重建应使用 release.json 记录的同一 Go 工具链。

默认生成：

- webcp-1.0.0-linux-amd64.tar.gz
- webcp-1.0.0-linux-arm64.tar.gz
- webcp-1.0.0-source.tar.gz
- release.json：版本、来源提交、Go 工具链、每个包的摘要与大小。
- SHA256SUMS：归档和 release.json 的 SHA-256。

文件名由实际 VERSION 决定。二进制包含程序、独立安装说明、空密钥配置示例、GPL/NOTICE 和依赖清单。对应源码包包含构建脚本、前端及后端源码、上游补丁和文档；按明确清单排除 data、bin、node_modules、Web 构建缓存、日志、测试报告和真实 webcp.json。脚本拒绝覆盖已有同名发布产物；重做候选包使用新输出目录，例如 `make release RELEASE_DIR=dist/candidate-2`。

发布二进制使用 CGO_ENABLED=0；Go SQLite 驱动无需宿主机 SQLite 动态库。通过 `-trimpath` 排除构建机路径，版本由 scripts/build.py 注入。若提供相同输入、工具链和 SOURCE_DATE_EPOCH，打包步骤固定文件顺序、权限、所有者和 gzip 时间；这不等于签名或第三方构建来源证明。

## GitHub 工作流

CI 使用 pull_request、分支 push、手动或复用调用，执行同一套本地入口。Release packages 在标签 push 或手动执行时先通过 CI，再构建两个 Linux 包及源码包，上传本次 run 的 artifact。工作流仅有 contents:read，不创建标签、不推送提交、不发布 GitHub Release。

官方 actions 固定完整 SHA，默认不保存 checkout 凭据。该策略遵循 [GitHub Actions 安全参考](https://docs.github.com/en/actions/reference/security/secure-use)。当前目录没有远端执行记录时，YAML/actionlint 和本地命令的通过不能写成“GitHub 云端已验证”。

正式发布由维护者检查 artifact、摘要、来源提交、许可和升级结果后执行。先建立 draft 并完整上传同次构建的归档、源码包、release.json 和 SHA256SUMS，再发布；已发布版本不覆盖附件，修复使用新版本。项目不自动提供 OIDC 构建证明，不能将 SHA256SUMS 当成签名。

## 升级验证与交付记录

至少验证新目录启动、未配置密钥拒绝启动、版本查询不依赖 Docker、旧 JSON 迁移、SQLite 重启、备份恢复、无效配置保留旧部署、已有容器主配置备份与页面同步。回退必须成套还原升级前的数据目录与二进制，详见 [安装说明](installation.md)。

本次检查结果、已修复项和未覆盖范围记录在 [发布检查记录](release-audit.md)。
