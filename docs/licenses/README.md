# 依赖许可证

先生成前端资源后运行 `make notices`，或执行完整前端构建，可从实际链接的 Go 模块、Go 标准库和已安装的前端构建依赖收集版本、来源及许可证，输出到 web/public/licenses/DEPENDENCIES.json 和 THIRD_PARTY_NOTICES.generated.txt。依赖图清单不等于所有列出的 npm 构建包都被嵌入浏览器代码。

原始许可文件按原文保留并记录 SHA-256。少数 npm 包只有 package.json 中的 MIT/ISC 声明，没有单独许可文件；清单明确标记 noticeSource、发布包 URL、完整性信息和声明作者，并附标准许可条款，不编造上游版权年份。标准条款来自 SPDX license-list-data 的 MIT.txt 和 ISC.txt，缺失的版权信息通过清单中的原始包元数据标识。

1Panel 源文件及补丁沿用 GPL-3.0 来源记录，根目录 LICENSE 与 NOTICE 随二进制和对应源码一起打包。旧版直接依赖的原始文本仍保留在本目录和 web/public/licenses 中。
