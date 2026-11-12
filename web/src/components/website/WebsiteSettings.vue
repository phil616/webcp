<script setup lang="ts">
import { defineAsyncComponent, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import type { Site } from '@/types';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import MainDiv from '@/vendor/1panel/components/main-div/index.vue';
import Status from '@/vendor/1panel/components/status/index.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import { useWebsiteSection } from './shared';
import { usePanelUI } from '@/composables/usePanelUI';
const props = defineProps<{ site: Site; initialTab?: string }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const { isMobile } = usePanelUI();
const nameOpen = ref(false),
  name = ref(props.site.name);
function rename() {
  resetRevision();
  name.value = props.site.name;
  nameOpen.value = true;
}
async function saveName() {
  if (await save('name', { name: name.value })) nameOpen.value = false;
}
const sections = [
  {
    id: 'basic',
    label: '域名设置',
    component: defineAsyncComponent(() => import('./DomainSettings.vue')),
  },
  {
    id: 'directory',
    label: '网站目录',
    component: defineAsyncComponent(() => import('./DirectorySettings.vue')),
  },
  {
    id: 'index',
    label: '默认文档',
    component: defineAsyncComponent(() => import('./DocumentSettings.vue')),
  },
  {
    id: 'proxy',
    label: '反向代理',
    component: defineAsyncComponent(() => import('./ProxySettings.vue')),
  },
  {
    id: 'cors',
    label: '跨域设置',
    component: defineAsyncComponent(() => import('./CORSSettings.vue')),
  },
  {
    id: 'tls',
    label: 'HTTPS',
    component: defineAsyncComponent(() => import('./HTTPSSettings.vue')),
  },
  {
    id: 'realip',
    label: '真实 IP',
    component: defineAsyncComponent(() => import('./RealIPSettings.vue')),
  },
  {
    id: 'redirect',
    label: '重定向',
    component: defineAsyncComponent(() => import('./RedirectSettings.vue')),
  },
  {
    id: 'config',
    label: '配置文件',
    component: defineAsyncComponent(() => import('./ConfigSettings.vue')),
  },
  {
    id: 'advanced',
    label: '响应头与安全',
    component: defineAsyncComponent(() => import('./AdvancedSettings.vue')),
  },
  {
    id: 'pages',
    label: '错误页面',
    component: defineAsyncComponent(() => import('./ExtraSettings.vue')),
  },
  {
    id: 'lua',
    label: 'Lua 扩展',
    component: defineAsyncComponent(() => import('./ExtraSettings.vue')),
  },
];
const route = useRoute(),
  router = useRouter();
const section = ref(sections.some((s) => s.id === props.initialTab) ? props.initialTab! : 'basic');
watch(
  () => route.query.tab,
  (value) => {
    const tab = String(value ?? 'basic');
    if (sections.some((s) => s.id === tab)) section.value = tab;
  },
);
watch(section, (value) => {
  if (route.query.tab !== value)
    void router.push({
      name: 'Website',
      query: { ...route.query, site: props.site.id, tab: value },
    });
});
</script>
<template>
  <LayoutContent :title="`网站设置 · ${site.name}`" back-name="sites"
    ><template #rightToolBar
      ><el-button type="primary" link :disabled="busy" @click="rename">修改名称</el-button
      ><Status :status="site.enabled ? 'enable' : 'disable'" /><el-tag effect="plain">{{
        site.domains.join(' · ')
      }}</el-tag></template
    ><template #main
      ><MainDiv :height-diff="265"
        ><el-tabs v-model="section" :tab-position="isMobile ? 'top' : 'left'" class="config-tabs"
          ><el-tab-pane
            v-for="tab in sections"
            :key="tab.id"
            :label="tab.label"
            :name="tab.id"
            :disabled="busy"
            ><component
              v-if="section === tab.id"
              :is="tab.component"
              :site="site"
              v-bind="
                tab.id === 'pages' || tab.id === 'lua' ? { section: tab.id } : {}
              " /></el-tab-pane></el-tabs></MainDiv></template
  ></LayoutContent>
  <PanelDrawer v-if="nameOpen" title="修改网站名称" @close="nameOpen = false"
    ><el-form label-position="top" @submit.prevent="saveName"
      ><el-form-item label="网站名称"
        ><el-input v-model="name" maxlength="128" /></el-form-item></el-form
    ><template #footer
      ><el-button @click="nameOpen = false">取消</el-button
      ><el-button type="primary" :loading="busy" @click="saveName">保存</el-button></template
    ></PanelDrawer
  >
</template>
