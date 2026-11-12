import { computed, defineComponent, h, defineAsyncComponent } from 'vue';
import { useRoute } from 'vue-router';
import { panelRouter } from '@panel/utils/router';
import { setBridge } from '@panel/api';
import type { App } from 'vue';
import Components from '@panel/components';
import BackButton from '@panel/components/back-button/index.vue';
import Status from '@panel/components/status/index.vue';
import SvgIcon from '@panel/components/svg-icon/svg-icon.vue';
import '@panel/assets/iconfont/iconfont.js';
import MainDiv from '@panel/components/main-div/index.vue';
import * as icons from '@element-plus/icons-vue';

export { setBridge, panelRouter };
export { KeyTypes as OfficialKeyTypes } from '@panel/global/mimetype';
export { default as OfficialOpDialog } from '@panel/components/del-dialog/index.vue';
export { default as OfficialRouterButton } from '@panel/components/router-button/index.vue';
export { default as OfficialLayout } from '@panel/components/layout-content/index.vue';
export { default as OfficialDrawer } from '@panel/components/drawer-pro/index.vue';
export { default as OfficialHTTPS } from '@panel/views/website/website/config/basic/https/index.vue';
const SSL = defineAsyncComponent(() => import('@panel/views/website/ssl/index.vue'));
const CA = defineAsyncComponent(() => import('@panel/views/website/ssl/ca/index.vue'));
const ACME = defineAsyncComponent(() => import('@panel/views/website/ssl/acme-account/index.vue'));
const DNS = defineAsyncComponent(() => import('@panel/views/website/ssl/dns-account/index.vue'));
export const CAModule = CA;
export const SSLModule = defineComponent({
    setup() {
        const route = useRoute();
        const view = computed(() =>
            route.path.endsWith('/ca')
                ? CA
                : route.path.endsWith('/acme-account')
                  ? ACME
                  : route.path.endsWith('/dns-account')
                    ? DNS
                    : SSL,
        );
        return () => h(view.value);
    },
});
export function install(app: App) {
    app.use(Components);
    Object.entries({ BackButton, Status, MainDiv, SvgIcon, ...icons }).forEach(([key, value]) =>
        app.component(key, value),
    );
}
