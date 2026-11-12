<script setup lang="ts">
import { computed, provide, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { logout } from './auth';
import { navigate } from './navigation';
import zhCn from 'element-plus/es/locale/lang/zh-cn';
import {
  Connection,
  Link,
  Document,
  Files,
  Key,
  Box,
  Fold,
  Refresh,
  Moon,
  Sunny,
  Setting,
} from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import { useManager, request, notify } from './api';
import { usePanelUI } from './composables/usePanelUI';
import PrimaryMenu from '@panel/assets/images/menu-bg.svg?component';
import PanelLayout from '@panel/layout/index.vue';
import AppMain from '@panel/layout/components/AppMain.vue';
import SubItem from '@panel/layout/components/Sidebar/components/SubItem.vue';
import { router } from './router';
import RouterButton from './components/panel/RouterButton.vue';
import Status from './vendor/1panel/components/status/index.vue';
import DeleteDialog from './components/panel/DeleteDialog.vue';
const appVersion = __APP_VERSION__;
const { overview, state, runtime, busy, notice, refresh, act } = useManager();
const { isCollapsed, isDark, isMobile, isFullScreen, mobileOpen, themeConfig } = usePanelUI();
const activeRoute = useRoute();
const current = computed(() => String(activeRoute.meta.section ?? 'sites'));
const failed = ref(false);
const confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const routes = [
  { id: 'sites', label: '网站', icon: Link },
  { id: 'certificates', label: 'SSL 证书', icon: Key },
  { id: 'mtls', label: 'mTLS', icon: Key },
  { id: 'lua', label: 'Lua 模块', icon: Box },
  { id: 'pages', label: '默认页面', icon: Document },
  { id: 'streams', label: '四层代理', icon: Connection },
  { id: 'runtime', label: 'OpenResty', icon: Setting },
  { id: 'backups', label: '备份与恢复', icon: Files },
];
const route = computed(() => routes.find((r) => r.id === current.value) ?? routes[0]);
const menuPaths: Record<string, string> = {
  sites: '/sites',
  certificates: '/websites/ssl',
  mtls: '/mtls',
  lua: '/lua',
  pages: '/pages',
  streams: '/streams',
  runtime: '/runtime',
  backups: '/backups',
};
const menuRecords = routes.map((item) => ({
  ...router.options.routes.find((r) => r.path === menuPaths[item.id])!,
  meta: {
    title: 'resty.menu.' + item.id,
    icon:
      item.id === 'sites'
        ? 'p-website'
        : item.id === 'certificates'
          ? 'p-ssl-menu'
          : item.id === 'runtime'
            ? 'p-xsetting-menu'
            : item.id === 'backups'
              ? 'p-file-menu'
              : undefined,
  },
}));
const menus = [
  {
    path: '/website-group',
    name: 'WebsiteGroup',
    redirect: '/sites',
    meta: { title: 'menu.website', icon: 'p-website' },
    children: menuRecords.slice(0, 6),
  },
  ...menuRecords.slice(6),
];
const activeMenu = computed(() => menuPaths[current.value]);
const websiteRoutes = routes.slice(0, 6);
const websiteSection = computed(() => websiteRoutes.some((r) => r.id === route.value.id));
const collapsed = computed(() => isCollapsed.value && !isMobile.value);
const dirty = computed(() => state.value?.revision !== runtime.value?.appliedRevision);
watch(
  () => activeRoute.fullPath,
  () => {
    mobileOpen.value = false;
  },
);
function select(id: string) {
  void navigate(id);
  mobileOpen.value = false;
}
async function signOut() {
  try {
    await logout();
  } catch {
    notify('退出登录失败，请重试', true);
  }
}
async function reload() {
  try {
    await refresh();
    failed.value = false;
  } catch (error) {
    failed.value = true;
    notify(String(error), true);
  }
}
async function apply() {
  await act(() => request('/runtime/apply', 'POST'), '配置已校验并应用。');
}
async function stop() {
  if (
    await confirmRef.value?.ask({
      title: '停止 OpenResty',
      message: '停止后，所有网站和四层代理将无法访问。是否继续？',
      names: [runtime.value?.container ?? 'OpenResty'],
    })
  )
    await act(() => request('/runtime/stop', 'POST'), 'OpenResty 已停止。');
}
watch(notice, (value) => {
  ElMessage.closeAll();
  if (value)
    ElMessage({
      type: value.error ? 'error' : 'success',
      message: value.text,
      duration: value.error ? 10000 : 3500,
      showClose: true,
    });
});
const viewRevision = ref(0);
async function reloadPage() {
  await reload();
  if (
    current.value === 'certificates' ||
    (current.value === 'mtls' && !/\/(clients|bindings)$/.test(activeRoute.path))
  )
    viewRevision.value++;
}
provide('reload', reloadPage);
let timer: ReturnType<typeof setInterval>;
onMounted(() => {
  reload();
  timer = setInterval(() => {
    if (!busy.value) reload();
  }, 30000);
});
onUnmounted(() => {
  clearInterval(timer);
});
</script>
<template>
  <el-config-provider :locale="zhCn" :empty-values="[undefined, null]"
    ><PanelLayout>
      <template #sidebar>
        <div class="sidebar-container">
          <div v-if="!collapsed" class="menu-background"><PrimaryMenu /></div>
          <div class="sidebar-title"><span v-if="!collapsed">WebCP</span></div>
          <el-scrollbar class="sidebar-scroll"
            ><el-menu
              :default-active="activeMenu"
              :default-openeds="['/website-group']"
              :collapse="collapsed"
              :collapse-transition="false"
              @select="
                (path) => {
                  void router.push(path);
                  mobileOpen = false;
                }
              "
              ><SubItem :menu-list="menus" :level="0" /></el-menu
          ></el-scrollbar>
          <div class="sidebar-footer">
            <el-button link @click="signOut">退出登录</el-button>
            <div class="sidebar-connection">
              <span class="connection-dot" /><span>本地节点</span>
            </div>
            <div class="engine-label">WebCP v{{ appVersion }}</div>
            <el-button
              link
              @click="themeConfig.theme = isDark ? 'light' : 'dark'"
              :aria-label="isDark ? '切换浅色主题' : '切换深色主题'"
              ><el-icon><Sunny v-if="isDark" /><Moon v-else /></el-icon
              ><span v-if="!collapsed">{{ isDark ? '浅色模式' : '深色模式' }}</span></el-button
            >
            <el-dropdown
              @command="(value) => (themeConfig.theme = String(value) as 'auto' | 'light' | 'dark')"
              trigger="click"
              ><el-button link aria-label="主题模式"
                >主题：{{
                  themeConfig.theme === 'auto' ? '跟随系统' : isDark ? '深色' : '浅色'
                }}</el-button
              ><template #dropdown
                ><el-dropdown-menu
                  ><el-dropdown-item command="light">浅色</el-dropdown-item
                  ><el-dropdown-item command="dark">深色</el-dropdown-item
                  ><el-dropdown-item command="auto">跟随系统</el-dropdown-item></el-dropdown-menu
                ></template
              ></el-dropdown
            >
          </div>
        </div> </template
      ><template #header
        ><header class="mobile-header">
          <el-button text aria-label="打开导航" @click="mobileOpen = !mobileOpen"
            ><el-icon :size="20"><Fold /></el-icon
          ></el-button>
          <h1>{{ route.label }}</h1>
          <el-button text aria-label="刷新" :disabled="busy" @click="reloadPage"
            ><el-icon><Refresh /></el-icon
          ></el-button>
        </header>
        <header class="page-header-bar">
          <el-breadcrumb separator="/"
            ><el-breadcrumb-item>{{ websiteSection ? '网站管理' : '服务管理' }}</el-breadcrumb-item
            ><el-breadcrumb-item
              ><h1>{{ route.label }}</h1></el-breadcrumb-item
            ></el-breadcrumb
          >
          <div class="page-header-actions">
            <span class="panel-group-label"><span class="connection-dot" /> 本地节点</span
            ><el-button
              link
              :icon="Refresh"
              aria-label="刷新"
              :disabled="busy"
              @click="reloadPage"
            />
          </div>
        </header>
      </template>
      <RouterButton
        v-if="!['certificates', 'mtls'].includes(route.id)"
        :active="route.id"
        :buttons="websiteSection ? websiteRoutes : [{ id: route.id, label: route.label }]"
      />
      <template v-if="overview"
        ><el-card v-if="!['certificates', 'mtls'].includes(route.id)" class="engine-card app-status"
          ><div class="engine-row">
            <span class="engine-name">OpenResty</span
            ><Status :status="runtime?.running ? 'running' : 'stopped'" /><span
              class="engine-image"
              :title="runtime?.image"
              >{{ runtime?.image }}</span
            >
            <div class="engine-actions">
              <span class="revision"
                >草稿 v{{ state?.revision }} / 已应用 v{{ runtime?.appliedRevision ?? 0 }}</span
              ><el-button
                v-if="!runtime?.running"
                type="primary"
                link
                :disabled="busy || !runtime?.available"
                @click="apply"
                >启动</el-button
              ><el-button v-else type="primary" link :disabled="busy" @click="stop">停止</el-button
              ><el-button
                type="primary"
                link
                :disabled="busy || !runtime?.available"
                @click="apply"
                >{{ runtime?.running ? '重载' : '校验并应用' }}</el-button
              ><el-button type="primary" link @click="select('runtime')">设置</el-button>
            </div>
          </div></el-card
        >
        <el-alert
          v-if="dirty && !['certificates', 'mtls'].includes(route.id)"
          class="pending-config"
          type="info"
          :closable="false"
          show-icon
          ><template #title
            ><span
              >存在未应用的配置。保存后，运行中的 OpenResty
              会自动校验并应用；四层监听变化会重建容器，服务停止时请手动启动。</span
            ></template
          ></el-alert
        >
        <AppMain :key="viewRevision" /> </template
      ><el-card v-else class="mt-3"
        ><div class="empty-service" v-loading="!failed">
          <el-empty v-if="failed" description="无法连接管理服务"
            ><el-button type="primary" @click="reloadPage">重新连接</el-button></el-empty
          ><span v-else>正在加载…</span>
        </div></el-card
      >
      <template #footer
        ><footer class="app-footer">
          WebCP · Web 控制面板
          <a href="https://github.com/1Panel-dev/1Panel" target="_blank" rel="noreferrer"
            >界面组件源自 1Panel</a
          >
        </footer></template
      ></PanelLayout
    ><DeleteDialog ref="confirmRef"
  /></el-config-provider>
</template>
<style scoped lang="scss">
@use '@panel/layout/components/Sidebar/index.scss';
</style>
