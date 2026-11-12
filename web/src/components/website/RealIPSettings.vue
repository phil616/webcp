<script setup lang="ts">
import { ref } from 'vue';
import type { Site } from '@/types';
import { lines, useWebsiteSection } from './shared';
const props = defineProps<{ site: Site }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const enabled = ref(props.site.realIp.enabled),
  from = ref(props.site.realIp.from?.join('\n') || '127.0.0.1'),
  header = ref(props.site.realIp.header || 'X-Real-IP'),
  other = ref('');
const headers = ['X-Forwarded-For', 'X-Real-IP', 'CF-Connecting-IP'];
if (!headers.includes(header.value)) {
  other.value = header.value;
  header.value = 'other';
}
function reset() {
  enabled.value = props.site.realIp.enabled;
  from.value = props.site.realIp.from?.join('\n') || '127.0.0.1';
  const value = props.site.realIp.header || 'X-Real-IP';
  header.value = headers.includes(value) ? value : 'other';
  other.value = value;
  resetRevision();
}
</script>
<template>
  <el-form label-position="top" class="form-width" @submit.prevent
    ><el-alert
      class="mb-4"
      type="info"
      show-icon
      :closable="false"
      title="只信任实际反向代理或 CDN 的 IP/CIDR。启用后递归读取指定请求头，将最后一个非信任地址作为真实 IP。"
    /><el-form-item label="启用真实 IP"><el-switch v-model="enabled" /></el-form-item
    ><template v-if="enabled"
      ><el-form-item label="信任来源"
        ><el-input
          v-model="from"
          type="textarea"
          :rows="8"
          placeholder="每行一个 IP 或 CIDR" /></el-form-item
      ><el-form-item label="IP Header"
        ><el-select v-model="header"
          ><el-option
            v-for="value in headers"
            :key="value"
            :value="value"
            :label="value" /><el-option value="other" label="其他" /></el-select></el-form-item
      ><el-form-item v-if="header === 'other'" label="自定义请求头"
        ><el-input v-model="other" /></el-form-item></template
    ><el-button
      type="primary"
      :loading="busy"
      @click="
        save('real-ip', { enabled, from: lines(from), header: header === 'other' ? other : header })
      "
      >保存</el-button
    ><el-button :disabled="busy" @click="reset">重置</el-button></el-form
  >
</template>
