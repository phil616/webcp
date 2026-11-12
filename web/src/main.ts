import { createApp, watch } from 'vue';
import { createPinia } from 'pinia';
import ElementPlus from 'element-plus';
import 'element-plus/dist/index.css';
import 'element-plus/theme-chalk/dark/css-vars.css';
import i18n from './i18n';
import { router } from './router';
import { install as installPanel } from '@panel/entry';
import { useInterfaceStore, persistPreference } from './stores/interface';
import './onepanel';
import App from './App.vue';
import { useTheme } from '@panel/global/use-theme';
import '@panel/styles/element.scss';
import '@panel/styles/element-dark.scss';
import '@panel/styles/fu.scss';
import '@panel/styles/mobile.scss';
import './style.scss';
import './styles-utilities.css';
const app = createApp(App);
app.use(createPinia());
app.use(i18n);
app.use(ElementPlus);
app.use(router);
installPanel(app);
const ui = useInterfaceStore();
const { switchTheme } = useTheme();
watch(
  () => ui.themeConfig.theme,
  (value) => {
    switchTheme();
    persistPreference('resty-theme', value);
  },
  { immediate: true },
);
watch(
  () => ui.isCollapsed,
  (value) => persistPreference('resty-menu-collapsed', String(value)),
);
router.isReady().then(() => app.mount('#app'));
