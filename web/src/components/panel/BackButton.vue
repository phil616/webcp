<!-- SPDX-License-Identifier: GPL-3.0-only
Adapted from 1Panel frontend/src/components/back-button/index.vue (e6b2aefdd1533870c80943db902f26338fbfd494).
Use the shared Vue Router for standalone navigation; keep el-page-header and its slot contract.
-->
<template>
  <el-page-header :content="header" title="返回" @back="jump"
    ><template v-if="slots.buttons" #content><slot name="buttons" /></template
  ></el-page-header>
</template>
<script setup lang="ts">
import { navigate } from '@/navigation';
import { useSlots } from 'vue';
const slots = useSlots();
const props = defineProps<{
  path?: string;
  name?: string;
  to?: object;
  header?: string;
  reload?: boolean;
}>();
function jump() {
  if (props.reload) {
    location.reload();
    return;
  }
  void navigate(props.path || props.name || 'sites');
}
</script>
