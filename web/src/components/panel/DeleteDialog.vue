<!-- SPDX-License-Identifier: GPL-3.0-only
Adapted from 1Panel frontend/src/components/del-dialog/index.vue (66ab42f650266919a6e3b082fb0eea9c60190e8c).
Preserves warning/names/footer confirmation flow; promise ownership is local and API calls remain in the service layer.
-->
<script setup lang="ts">
import { ref } from 'vue';
import DialogPro from '@/vendor/1panel/components/dialog-pro/index.vue';
const open = ref(false);
const title = ref('删除');
const message = ref('');
const names = ref<string[]>([]);
let resolve: ((confirmed: boolean) => void) | undefined;
function ask(options: { title?: string; message: string; names?: string[] }) {
  if (resolve) resolve(false);
  title.value = options.title ?? '删除';
  message.value = options.message;
  names.value = options.names ?? [];
  open.value = true;
  return new Promise<boolean>((done) => (resolve = done));
}
function finish(result: boolean) {
  open.value = false;
  resolve?.(result);
  resolve = undefined;
}
defineExpose({ ask });
</script>
<template>
  <DialogPro v-model="open" :title="title" size="small" @close="finish(false)"
    ><el-alert :closable="false" show-icon type="warning" :title="message" />
    <ul class="delete-names">
      <li v-for="name in names" :key="name">{{ name }}</li>
    </ul>
    <template #footer
      ><el-button @click="finish(false)">取消</el-button
      ><el-button type="primary" @click="finish(true)">确认</el-button></template
    ></DialogPro
  >
</template>
