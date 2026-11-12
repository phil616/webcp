## 网站配置行为参考

以下文件同样固定于 `3adee9445897aa41c0160b4f805a9b1f43f712ac`，用于本次设置模块的字段、表单流程、端口/证书约束、代理/CORS/重定向逻辑对照。新表单通过本地类型化 DTO 接入独立服务，不包含上游授权、节点和应用安装依赖。

| 上游路径 | Git blob SHA |
| --- | --- |
| frontend/src/views/website/website/config/basic/index.vue | db51089d969e2a78e4ad75d7d0a3fabc940502bf |
| frontend/src/views/website/website/config/basic/domain/index.vue | 9bccc1763cdde9acef7e61801b788b8c1bd865ac |
| frontend/src/views/website/website/config/basic/domain/create/index.vue | 084217a1634b10f5faee422c287382b9fb972b10 |
| frontend/src/views/website/website/config/basic/site-folder/index.vue | 771cedc31312046f211337ba1bcbec92e5776fd0 |
| frontend/src/views/website/website/config/basic/default-doc/index.vue | 630b7c950c0d6ec1ce9f1e09536108060cac1348 |
| frontend/src/views/website/website/config/basic/proxy/index.vue | ee495f07080ad1ee050d585d99795e0b8bca9aec |
| frontend/src/views/website/website/config/basic/proxy/create/index.vue | faf5e9fda4318a12928d547a8ca0f0cbbaf90802 |
| frontend/src/views/website/website/cors/index.vue | 5f492d211090fba766e3b654ba041a434cf76dd9 |
| frontend/src/views/website/website/config/basic/https/index.vue | 54c6352f7cac0aaedd61eb1ad6c1f5526c844b65 |
| frontend/src/views/website/website/components/https/index.vue | 0ef246f1e9caa6a3cb0c5651e21084877f56ad10 |
| frontend/src/views/website/website/config/basic/real-ip/index.vue | 3c9e679ae615f431102157707fab8b2d9d233de2 |
| frontend/src/views/website/website/config/basic/redirect/index.vue | f7f80924e7a20e4c8f5a34c9d3d150c8287cf003 |
| frontend/src/views/website/website/config/basic/redirect/create/index.vue | 9b0884a4c962f4ca4fed36da6b588bc545170f7c |
| agent/app/service/website_domain.go | fcc63a2cbff389c770a710599e0e682388cbf72f |
| agent/app/service/website_proxy.go | 2cbbccfd5d5db99307daaadad1d584d303b7f6af |
| agent/utils/nginx/components/location.go | eb2740d247a776c28ea2b52c5569c5888f0397eb |
