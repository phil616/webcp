<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Document, Plus, Upload } from '@element-plus/icons-vue';
import { useManager, request, size } from '@/api';
import type { Site, SiteFile } from '@/types';
import PanelDrawer from './PanelDrawer.vue';
import DeleteDialog from './DeleteDialog.vue';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const props = defineProps<{ site: Site }>();
const emit = defineEmits<{ close: [] }>();
const { overview, busy, act } = useManager();
const files = ref<SiteFile[]>([]),
  path = ref('index.html'),
  content = ref(''),
  editable = ref(true);
const confirmRef = ref<InstanceType<typeof DeleteDialog>>();
async function load() {
  files.value = await request<SiteFile[]>(`/sites/${props.site.id}/files`);
}
async function openFile(name: string) {
  await act(async () => {
    editable.value = false;
    const value = await request<{ content: string }>(
      `/sites/${props.site.id}/file?path=${encodeURIComponent(name)}`,
    );
    path.value = name;
    content.value = value.content;
    editable.value = true;
  }, '文件已读取');
}
onMounted(async () => {
  await act(load, '目录已读取');
  if (files.value.some((file) => file.path === 'index.html')) await openFile('index.html');
});
function newFile() {
  path.value = 'new.html';
  content.value = '';
  editable.value = true;
}
async function save() {
  await act(async () => {
    await request(`/sites/${props.site.id}/file`, 'PUT', {
      path: path.value,
      content: content.value,
    });
    await load();
  }, '文件已保存，即时生效。');
}
async function remove() {
  if (
    await confirmRef.value?.ask({
      title: '删除文件',
      message: '此操作将立即删除网站文件，无法撤销。',
      names: [path.value],
    })
  ) {
    await act(async () => {
      await request(
        `/sites/${props.site.id}/file?path=${encodeURIComponent(path.value)}`,
        'DELETE',
      );
      await load();
      newFile();
    }, '删除成功');
  }
}
async function upload(event: Event) {
  const input = event.target as HTMLInputElement;
  const selected = Array.from(input.files ?? []);
  await act(async () => {
    for (const file of selected) {
      const response = await fetch(
        `/api/sites/${props.site.id}/upload?path=${encodeURIComponent(file.name)}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/octet-stream',
            'If-Match': String(overview.value!.state.revision),
          },
          body: file,
        },
      );
      if (!response.ok) throw new Error((await response.json()).error);
    }
    await load();
  }, '上传成功');
  input.value = '';
}
</script>
<template>
  <PanelDrawer title="网站目录" :resource="site.name" size="60%" @close="emit('close')"
    ><el-alert
      :closable="false"
      type="info"
      show-icon
      title="文件修改即时生效。支持 UTF-8 文本编辑与二进制文件上传，单文件上限 4 MiB。"
      class="mb-4"
    />
    <div class="header-tools mb-4">
      <el-button :icon="Plus" @click="newFile">新建文件</el-button>
      <div class="upload-control">
        <el-button :icon="Upload" :disabled="busy" tabindex="-1" aria-hidden="true"
          >上传文件</el-button
        ><input type="file" aria-label="上传文件" multiple :disabled="busy" @change="upload" />
      </div>
    </div>
    <div class="file-layout">
      <aside class="file-list">
        <el-button
          v-for="file in files"
          :key="file.path"
          :class="{ selected: file.path === path }"
          text
          :disabled="busy"
          @click="openFile(file.path)"
          ><el-icon class="mr-1"><Document /></el-icon><span>{{ file.path }}</span
          ><small>{{ size(file.size) }}</small></el-button
        >
      </aside>
      <div class="file-editor">
        <el-form label-position="top"
          ><el-form-item label="文件路径"
            ><el-input v-model="path" placeholder="assets/style.css" /></el-form-item></el-form
        ><CodemirrorPro
          v-model="content"
          :readonly="!editable"
          :mode="
            path.endsWith('.html')
              ? 'html'
              : path.endsWith('.css')
                ? 'css'
                : path.endsWith('.js')
                  ? 'javascript'
                  : 'text'
          "
          label="文件内容"
          :height="430"
        />
      </div>
    </div>
    <template #footer
      ><el-button
        type="danger"
        plain
        :disabled="busy || !files.some((file) => file.path === path)"
        @click="remove"
        >删除文件</el-button
      ><el-button :disabled="busy" @click="emit('close')">关闭</el-button
      ><el-button type="primary" :disabled="!editable" :loading="busy" @click="save"
        >保存文件</el-button
      ></template
    ></PanelDrawer
  ><DeleteDialog ref="confirmRef" />
</template>
