<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import zhCn from 'element-plus/es/locale/lang/zh-cn';
import { authenticated, checkingSession, checkSession } from './auth';
import { clearManagerSession } from './api';
import ManagerApp from './ManagerApp.vue';
import LoginView from './views/LoginView.vue';
const error = ref('');
watch(authenticated, (value) => {
  if (!value) clearManagerSession();
});
onMounted(() =>
  checkSession().catch(() => {
    error.value = '无法连接 WebCP，请检查服务后重试。';
  }),
);
</script>
<template>
  <el-config-provider :locale="zhCn"
    ><div
      v-if="checkingSession"
      class="session-loading"
      v-loading="true"
      aria-label="检查登录状态" />
    <ManagerApp v-else-if="authenticated" /><LoginView v-else :connection-error="error"
  /></el-config-provider>
</template>
<style scoped>
.session-loading {
  height: 100dvh;
}
</style>
