<script setup lang="ts">
import { ref } from 'vue';
import { login } from '@/auth';
defineProps<{ connectionError?: string }>();
const key = ref(''),
  loading = ref(false),
  error = ref('');
async function submit() {
  if (!key.value || loading.value) return;
  loading.value = true;
  error.value = '';
  try {
    await login(key.value);
    key.value = '';
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '登录失败';
  } finally {
    loading.value = false;
  }
}
</script>
<template>
  <main class="login-page">
    <el-card class="login-card"
      ><h1>WebCP</h1>
      <p class="login-description">Web 控制面板</p>
      <el-alert
        v-if="error || connectionError"
        :title="error || connectionError"
        type="error"
        :closable="false"
        show-icon
        class="mb-4"
      /><el-form label-position="top" @submit.prevent="submit"
        ><el-form-item label="访问密钥"
          ><el-input
            v-model="key"
            type="password"
            show-password
            autocomplete="current-password"
            placeholder="输入配置中提供的访问密钥"
            :disabled="loading"
            autofocus /></el-form-item
        ><el-button
          type="primary"
          native-type="submit"
          :loading="loading"
          :disabled="!key"
          class="login-submit"
          >登录</el-button
        ></el-form
      ></el-card
    >
  </main>
</template>
<style scoped lang="scss">
.login-page {
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--panel-main-bg-color-9);
}
.login-card {
  width: 420px;
  max-width: 100%;
  --el-card-padding: 32px;
}
h1 {
  font-size: 26px;
  font-weight: 500;
  margin-bottom: 8px;
}
.login-description {
  color: var(--el-text-color-secondary);
  margin-bottom: 28px;
}
.login-submit {
  width: 100%;
}
</style>
