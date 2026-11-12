<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/table/TableRefresh.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <el-button class="fresh-button" icon="Refresh" @click="onRefresh"></el-button>
</template>

<script setup lang="ts">
defineOptions({ name: 'TableRefresh' });

const emit = defineEmits(['search']);

const onRefresh = () => {
  emit('search');
};
</script>

<style lang="scss" scoped>
.fresh-button {
  float: right;
}
</style>
