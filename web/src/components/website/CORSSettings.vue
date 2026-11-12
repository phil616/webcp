<script setup lang="ts">
import { ref } from 'vue';
import type { Site } from '@/types';
import { clone, defaultCORS, useWebsiteSection } from './shared';
import CORSFields from './CORSFields.vue';
const props = defineProps<{ site: Site }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const config = ref({ ...defaultCORS(), ...clone(props.site.cors) });
function reset() {
  config.value = { ...defaultCORS(), ...clone(props.site.cors) };
  resetRevision();
}
</script>
<template>
  <el-form label-position="top" class="form-width" @submit.prevent="save('cors', config)"
    ><CORSFields v-model="config" /><el-button
      type="primary"
      :loading="busy"
      @click="save('cors', config)"
      >保存</el-button
    ><el-button :disabled="busy" @click="reset">重置</el-button></el-form
  >
</template>
