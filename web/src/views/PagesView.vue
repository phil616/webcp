<script setup lang="ts">
import { computed, ref, onMounted } from 'vue';
import { useManager, request, notify } from '@/api';
import type { PageDocument } from '@/types';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import ErrorPageEditor from '@/components/panel/ErrorPageEditor.vue';
const props = defineProps<{ drawer?: boolean }>();
const emit = defineEmits<{ close: [] }>();
const { state, busy, act } = useManager();
const pages = ref<Record<string, string>>({});
const original = ref('{}');
const etag = ref('');
const loading = ref(true);
const revision = ref(state.value!.revision);
const changed = computed(() => JSON.stringify(pages.value) !== original.value);
async function reset() {
  loading.value = true;
  try {
    const document = await request<PageDocument>('/pages');
    pages.value = document.pages;
    original.value = JSON.stringify(document.pages);
    etag.value = document.etag;
    revision.value = state.value!.revision;
  } catch (error) {
    notify(error instanceof Error ? error.message : String(error), true);
  } finally {
    loading.value = false;
  }
}
onMounted(reset);
async function save() {
  if (
    await act(
      () => request('/pages', 'PUT', { pages: pages.value, etag: etag.value }, revision.value),
      '页面文件已保存',
    )
  ) {
    await reset();
    if (props.drawer) emit('close');
  }
}
</script>
<template>
  <div class="route-view">
    <PanelDrawer v-if="drawer" title="高级设置" size="60%" @close="emit('close')"
      ><el-alert
        class="mb-4"
        type="info"
        show-icon
        :closable="false"
        title="直接编辑 OpenResty 默认页面文件，保存后即时生效；网站独立页面优先。"
      /><ErrorPageEditor v-if="etag && !loading" v-model="pages" /><template #footer
        ><el-button :disabled="busy" @click="emit('close')">取消</el-button
        ><el-button
          type="primary"
          :disabled="loading || !etag || !changed"
          :loading="busy"
          @click="save"
          >保存</el-button
        ></template
      ></PanelDrawer
    ><LayoutContent v-else
      ><template #leftToolBar
        ><el-button
          type="primary"
          :disabled="loading || !etag || !changed"
          :loading="busy"
          @click="save"
          >保存</el-button
        ><el-button :disabled="busy || loading || !changed" @click="reset"
          >重置</el-button
        ></template
      ><template #main
        ><p class="section-description">
          OpenResty 直接读取默认页面文件。保存后即时生效，无须重新应用配置；网站独立页面优先。
        </p>
        <ErrorPageEditor v-if="etag && !loading" v-model="pages" /></template
    ></LayoutContent>
  </div>
</template>
