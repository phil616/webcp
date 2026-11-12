<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { errorPages, defaultErrorPage } from '@/error-pages';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const pages = defineModel<Record<string, string>>({ required: true });
const code = ref('404');
const preview = ref(true);
const codes = errorPages;
watch(code, (value) => {
  if (pages.value[value] === undefined) pages.value[value] = defaultErrorPage(value);
});
const byteLength = computed(() => new TextEncoder().encode(pages.value[code.value] ?? '').length);
</script>
<template>
  <div class="header-tools mb-4">
    <el-select v-model="code" class="p-w-200" aria-label="选择错误页面"
      ><el-option
        v-for="item in codes"
        :key="item.id"
        :value="item.id"
        :label="item.label" /></el-select
    ><el-checkbox v-model="preview">显示预览</el-checkbox>
  </div>
  <div :class="{ 'page-edit-grid': preview }">
    <div>
      <div class="editor-label">
        <span>{{ code }}.html</span><small>HTML</small>
      </div>
      <CodemirrorPro v-model="pages[code]!" mode="html" label="错误页面 HTML" :height="400" />
      <div class="editor-foot">
        <span>{{ byteLength }} 字节 / 256 KiB</span
        ><span>OpenResty 页面文件，网站独立文件优先</span>
      </div>
    </div>
    <div v-if="preview">
      <div class="editor-label"><span>页面预览</span><small>脚本已隔离</small></div>
      <iframe
        class="page-preview"
        :srcdoc="pages[code]"
        sandbox=""
        title="错误页面预览"
        referrerpolicy="no-referrer"
      />
    </div>
  </div>
</template>
