<script setup lang="ts">
import { computed } from 'vue';
import OriginalRouterButton from '@panel/components/router-button/index.vue';
import { sectionPath } from '@/navigation';
const props = defineProps<{
  active: string;
  buttons: { id: string; label: string; disabled?: boolean }[];
}>();
const routes = computed(() =>
  props.buttons.map((button) => ({ ...button, path: sectionPath(button.id) })),
);
</script>
<template>
  <OriginalRouterButton :buttons="routes"
    ><template #route-button><slot name="route-button" /></template
  ></OriginalRouterButton>
</template>
