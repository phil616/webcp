<script setup lang="ts">
import type { CORSConfig } from '@/types';
import { defaultCORS } from './shared';
const config = defineModel<CORSConfig>({ required: true });
function change(value: boolean | string | number) {
  config.value = value
    ? { ...defaultCORS(), enabled: true }
    : {
        enabled: false,
        allowOrigins: '',
        allowMethods: '',
        allowHeaders: '',
        allowCredentials: false,
        preflight: true,
      };
}
</script>
<template>
  <el-form-item label="启用跨域"
    ><el-switch v-model="config.enabled" @change="change"
  /></el-form-item>
  <template v-if="config.enabled">
    <el-form-item label="允许的来源"
      ><div class="field-control">
        <el-input
          v-model="config.allowOrigins"
          type="textarea"
          :rows="3"
          placeholder="* 或每行一个 https://example.com"
        /><span class="input-help"
          >支持 *、明确的来源列表或 $http_origin。携带凭据时请指定来源，或明确选择
          $http_origin。</span
        >
      </div></el-form-item
    >
    <el-form-item label="允许的请求方法"
      ><el-input v-model="config.allowMethods" placeholder="GET,POST,OPTIONS,PUT,DELETE"
    /></el-form-item>
    <el-form-item label="允许的请求头"
      ><el-input v-model="config.allowHeaders" placeholder="Content-Type,Authorization"
    /></el-form-item>
    <el-form-item label="允许携带凭据"
      ><el-switch v-model="config.allowCredentials"
    /></el-form-item>
    <el-form-item label="预检请求"
      ><div class="field-control">
        <el-switch v-model="config.preflight" /><span class="input-help"
          >启用后，OPTIONS 请求直接返回 204。</span
        >
      </div></el-form-item
    >
  </template>
</template>
