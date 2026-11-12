export const errorPages = [
  { id: '403', label: '403 - 禁止访问' },
  { id: '404', label: '404 - 页面不存在' },
  { id: '495', label: '495 - 客户端证书校验失败' },
  { id: '496', label: '496 - 未提供客户端证书' },
  { id: '500', label: '500 - 服务器内部错误' },
  { id: '502', label: '502 - 网关错误' },
  { id: '503', label: '503 - 服务不可用' },
  { id: '504', label: '504 - 网关超时' },
];
export function defaultErrorPage(code: string) {
  const title = errorPages.find((page) => page.id === code)?.label ?? code;
  return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>${title}</title><body><h1>${title}</h1></body></html>`;
}
