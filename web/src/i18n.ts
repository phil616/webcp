import { createI18n } from 'vue-i18n';
import zh from '../panel/src/lang/modules/zh';
export default createI18n({
  legacy: false,
  locale: 'zh',
  fallbackLocale: 'zh',
  messages: {
    zh: {
      ...zh,
      resty: {
        menu: {
          sites: '网站',
          certificates: 'SSL 证书',
          mtls: 'mTLS',
          lua: 'Lua 模块',
          pages: '默认页面',
          streams: '四层代理',
          runtime: 'OpenResty',
          backups: '备份与恢复',
        },
      },
    } as unknown as Record<string, string>,
  },
});
