import { defineStore } from 'pinia';
function readPreference(key: string) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
export const useInterfaceStore = defineStore('interface', {
  state: () => ({
    currentNode: 'local',
    isMobile: false,
    isFullScreen: false,
    mobileOpen: false,
    withoutAnimation: false,
    themeConfig: {
      theme: (readPreference('resty-theme') || 'auto') as 'light' | 'dark' | 'auto',
      primary: '#005eeb',
      themeColor: '',
    },
    isCollapsed: readPreference('resty-menu-collapsed') === 'true',
    isDark: readPreference('resty-theme') === 'dark',
    isMaster: false,
    isProductPro: false,
    isXpackOrEE: false,
    isEnterprise: false,
    isIntl: false,
    productProExpires: 0,
    openMenuTabs: [] as { path: string; title?: string }[],
    cacheUserID: 'standalone',
    panelCache: {
      tabs: true,
      pagination: true,
      refresh: true,
      search: false,
      duration: '24h' as '24h' | 'week' | 'month' | 'forever',
    },
    terminalFontSize: 14,
    language: 'zh',
  }),
});
export function persistPreference(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Browser storage is optional. */
  }
}
