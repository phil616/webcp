<script setup lang="ts">
import { ref } from 'vue';
import type { Site } from '@/types';
import { lines, useWebsiteSection } from './shared';
const props = defineProps<{ site: Site }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const text = ref(props.site.index.join('\n'));
function reset() {
  text.value = props.site.index.join('\n');
  resetRevision();
}
</script>
<template>
  <el-form
    label-position="top"
    class="form-width"
    @submit.prevent="save('index', { index: lines(text) })"
    ><el-form-item label="默认文档"
      ><div class="field-control">
        <el-input
          v-model="text"
          type="textarea"
          :rows="8"
          placeholder="index.html&#10;index.htm"
        /><span class="input-help">每行一个文件名，按顺序查找；不支持 PHP 文档。</span>
      </div></el-form-item
    ><el-button type="primary" :loading="busy" @click="save('index', { index: lines(text) })"
      >保存</el-button
    ><el-button :disabled="busy" @click="reset">重置</el-button></el-form
  >
</template>
