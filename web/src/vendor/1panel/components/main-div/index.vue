<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/main-div/index.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <div class="main-div" :style="{ '--main-height': mainHeight + 'px' }">
    <slot></slot>
  </div>
</template>
<script lang="ts" setup>
import { ref, onMounted, onUnmounted, computed } from 'vue';
const props = defineProps({
  heightDiff: {
    type: Number,
    default: 0,
  },
});
const windowHeight = ref(window.innerHeight);
const mainHeight = computed(() => windowHeight.value - props.heightDiff);

const updateHeight = () => {
  windowHeight.value = window.innerHeight;
};

onMounted(() => {
  window.addEventListener('resize', updateHeight);
});

onUnmounted(() => {
  window.removeEventListener('resize', updateHeight);
});
defineOptions({ name: 'MainDiv' });
</script>
<style scoped>
.main-div {
  height: var(--main-height);
  overflow-y: auto;
  overflow-x: hidden;
}
</style>
