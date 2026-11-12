<script setup lang="ts">
import { onMounted, ref } from 'vue';
import EngineConnection from '@/components/panel/EngineConnection.vue';
import Performance from '@panel/views/website/website/nginx/performance/index.vue';
import { useManager, request, notify } from '@/api';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import CardWithHeader from '@/vendor/1panel/components/card-with-header/index.vue';
import TableRefresh from '@/vendor/1panel/components/table/TableRefresh.vue';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const appVersion = __APP_VERSION__;
const { runtime, state } = useManager();
const performanceRef = ref<InstanceType<typeof Performance>>();
const tab = ref('config'),
  config = ref(''),
  logs = ref(''),
  loading = ref(false);
async function load() {
  loading.value = true;
  try {
    if (tab.value === 'config')
      config.value = (await request<{ config: string }>('/config')).config;
    else if (tab.value === 'performance') await performanceRef.value?.reload();
    else if (tab.value === 'logs')
      logs.value = (await request<{ logs: string }>('/runtime/logs')).logs;
  } catch (error) {
    notify(String(error), true);
  } finally {
    loading.value = false;
  }
}
onMounted(load);
</script>
<template>
  <div class="route-view">
    <LayoutContent
      ><template #app
        ><CardWithHeader header="服务信息"
          ><template #body
            ><el-descriptions :column="2" class="runtime-description"
              ><el-descriptions-item label="容器名称">{{ runtime?.container }}</el-descriptions-item
              ><el-descriptions-item label="镜像">{{ runtime?.image }}</el-descriptions-item
              ><el-descriptions-item label="草稿版本">v{{ state?.revision }}</el-descriptions-item
              ><el-descriptions-item label="已应用版本"
                >v{{ runtime?.appliedRevision ?? 0 }}</el-descriptions-item
              ></el-descriptions
            ><el-alert
              v-if="runtime?.detail"
              :title="runtime.detail"
              :closable="false"
              type="info"
              show-icon
              class="mt-3" /></template></CardWithHeader></template
      ><template #leftToolBar
        ><el-button type="primary" :plain="tab !== 'connection'" @click="tab = 'connection'"
          >容器连接</el-button
        ><el-button type="primary" :plain="tab !== 'performance'" @click="tab = 'performance'"
          >性能调节</el-button
        ><el-button
          type="primary"
          :plain="tab !== 'config'"
          @click="
            tab = 'config';
            load();
          "
          >配置文件</el-button
        ><el-button
          type="primary"
          :plain="tab !== 'logs'"
          @click="
            tab = 'logs';
            load();
          "
          >日志</el-button
        ><el-button type="primary" :plain="tab !== 'about'" @click="tab = 'about'"
          >关于</el-button
        ></template
      ><template #rightToolBar
        ><TableRefresh
          v-if="['config', 'logs', 'performance'].includes(tab)"
          aria-label="刷新内容"
          :disabled="loading"
          @search="load" /></template
      ><template #main
        ><EngineConnection v-if="tab === 'connection'" />
        <Performance v-else-if="tab === 'performance'" ref="performanceRef" />
        <div v-else-if="tab === 'config'" v-loading="loading">
          <p class="section-description">
            当前草稿生成的只读配置预览。实际应用会生成独立版本目录，通过 OpenResty 与 LuaJIT
            校验后切换并重载。
          </p>
          <CodemirrorPro
            v-model="config"
            mode="nginx"
            readonly
            label="生成的 Nginx 配置"
            :height="500"
          />
        </div>
        <template v-else-if="tab === 'logs'"
          ><p class="section-description">最近 200 行容器输出，包含访问日志和错误日志。</p>
          <pre v-loading="loading" class="log-output" tabindex="0" aria-label="容器日志">{{
            logs || '暂无日志'
          }}</pre>
        </template>
        <div v-else>
          <el-descriptions :column="1" border
            ><el-descriptions-item label="程序">WebCP</el-descriptions-item
            ><el-descriptions-item label="界面来源"
              >复用 1Panel
              的布局、主题、抽屉、对话框、表格与编辑器组件；仅保留独立网站管理功能。</el-descriptions-item
            ><el-descriptions-item label="上游版本"
              ><span class="mono">1Panel 3adee9445897</span></el-descriptions-item
            ><el-descriptions-item label="许可证"
              ><a href="/licenses/GPL-3.0.txt" target="_blank" rel="noreferrer">GPL-3.0</a
              >，无任何担保。<a href="/licenses/NOTICE.txt" target="_blank" rel="noreferrer"
                >来源声明</a
              >与依赖许可证随程序分发。</el-descriptions-item
            ><el-descriptions-item label="项目链接"
              ><a href="https://github.com/1Panel-dev/1Panel" target="_blank" rel="noreferrer"
                >1Panel 上游项目</a
              ></el-descriptions-item
            ></el-descriptions
          >
        </div></template
      ></LayoutContent
    >
  </div>
</template>
