<script setup lang="ts">
import { computed, onMounted } from 'vue';
import DrawerPro from '@/vendor/1panel/components/drawer-pro/index.vue';
import { useManager } from '@/api';
const props = withDefaults(defineProps<{ title: string; resource?: string; size?: string }>(), {
  size: 'normal',
  resource: '',
});
const emit = defineEmits<{ close: [] }>();
const { busy, notice } = useManager();
onMounted(() => {
  notice.value = undefined;
});
const open = computed({
  get: () => true,
  set: () => {
    if (!busy.value) emit('close');
  },
});
</script>
<template>
  <DrawerPro
    v-model="open"
    :header="props.title"
    :resource="resource"
    :size="size"
    :disabled="busy"
    :auto-close="!busy"
    full-screen
    @close="emit('close')"
    ><template #content
      ><el-alert
        v-if="notice?.error"
        type="error"
        :closable="false"
        :title="notice.text"
        show-icon
        class="mb-4" /><slot /></template
    ><template #footer><slot name="footer" /></template
  ></DrawerPro>
</template>
