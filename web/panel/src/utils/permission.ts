// The original user scope excludes authentication and RBAC; upstream table gates are removed.
export type PermissionBindingValue = unknown;
export const hasManagePermissionAccess = (_permission?: unknown, _options?: object) => true;
export const hasPermissionAccess = (_permission?: unknown, _options?: object) => true;
