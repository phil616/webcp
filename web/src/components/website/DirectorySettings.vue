<script setup lang="ts">
import { onMounted, ref } from 'vue';
import type { Site } from '@/types';
import { notify, request } from '@/api';
import { useWebsiteSection } from './shared';
import WebsiteFiles from '@/components/panel/WebsiteFiles.vue';
const props = defineProps<{ site: Site }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const directory = ref(props.site.runDirectory),
  dirs = ref<string[]>(['/']),
  files = ref(false);
async function load() {
  try {
    dirs.value = await request<string[]>(`/sites/${props.site.id}/directories`);
  } catch (e) {
    notify(String(e), true);
  }
}
function reset() {
  directory.value = props.site.runDirectory;
  resetRevision();
  void load();
}
onMounted(load);
</script>
<template>
  <div class="form-width">
    <el-descriptions :column="1" border
      ><el-descriptions-item label="网站目录"
        ><span class="mono">sites/{{ site.id }}</span>
        <el-button type="primary" link @click="files = true"
          >管理文件</el-button
        ></el-descriptions-item
      ></el-descriptions
    ><el-form
      label-position="top"
      class="mt-4"
      @submit.prevent="save('directory', { runDirectory: directory })"
      ><el-form-item label="运行目录"
        ><div class="field-control">
          <el-select v-model="directory" filterable
            ><el-option v-for="dir in dirs" :key="dir" :label="dir" :value="dir" /></el-select
          ><span class="input-help">选择网站根目录或其真实子目录。上传文件后可刷新目录列表。</span>
        </div></el-form-item
      ><el-button
        type="primary"
        :loading="busy"
        @click="save('directory', { runDirectory: directory })"
        >保存并重载</el-button
      ><el-button :disabled="busy" @click="reset">重置 / 刷新目录</el-button></el-form
    >
  </div>
  <WebsiteFiles
    v-if="files"
    :site="site"
    @close="
      files = false;
      load();
    "
  />
</template>
