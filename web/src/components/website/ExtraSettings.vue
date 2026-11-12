<script setup lang="ts">
import { ref, onMounted } from 'vue';
import type { Site, PageDocument } from '@/types';
import { request, notify } from '@/api';
import { clone, useWebsiteSection } from './shared';
import WebsiteFields from '@/components/panel/WebsiteFields.vue';
const props = defineProps<{ site: Site; section: 'pages' | 'lua' }>();
const { busy, act, revision, resetRevision } = useWebsiteSection(() => props.site);
const draft = ref(clone(props.site)),
  domains = ref(props.site.domains.join('\n'));
const pages = ref<Record<string, string>>({}),
  globalPages = ref<Record<string, string>>({}),
  etag = ref(''),
  loading = ref(props.section === 'pages');
async function loadPages() {
  loading.value = true;
  try {
    const [own, global] = await Promise.all([
      request<PageDocument>(`/sites/${props.site.id}/pages`),
      request<PageDocument>('/pages'),
    ]);
    pages.value = own.pages;
    etag.value = own.etag;
    globalPages.value = global.pages;
    resetRevision();
  } catch (error) {
    notify(error instanceof Error ? error.message : String(error), true);
  } finally {
    loading.value = false;
  }
}
onMounted(() => {
  if (props.section === 'pages') void loadPages();
});
function reset() {
  if (props.section === 'pages') {
    void loadPages();
    return;
  }
  draft.value = clone(props.site);
  resetRevision();
}
async function save() {
  const value =
    props.section === 'pages'
      ? { pages: pages.value, etag: etag.value }
      : {
          luaBindings: Object.fromEntries(
            Object.entries(draft.value.luaBindings).filter(([, v]) => v),
          ),
        };
  if (
    await act(
      () =>
        request(
          `/sites/${props.site.id}/${props.section === 'pages' ? 'pages' : 'extensions'}`,
          'PUT',
          value,
          revision.value,
        ),
      '保存成功',
      { deploy: props.section !== 'pages' },
    )
  ) {
    if (props.section === 'pages') await loadPages();
    else resetRevision();
  }
}
</script>
<template>
  <div class="form-width">
    <el-form label-position="top" @submit.prevent="save"
      ><WebsiteFields
        v-if="!loading"
        v-model:draft="draft"
        v-model:domains="domains"
        v-model:pages="pages"
        :global-pages="globalPages"
        :section="section"
      />
      <div class="mt-4">
        <el-button
          type="primary"
          :loading="busy || loading"
          :disabled="section === 'pages' && !etag"
          @click="save"
          >保存</el-button
        ><el-button :disabled="busy" @click="reset">重置</el-button>
      </div></el-form
    >
  </div>
</template>
