<template>
    <RouterViewCache :keep-alive="keepAlive" with-transition />
</template>

<script setup lang="ts">
import type { KeepAlivePattern } from '@panel/utils/tab-cache';

defineProps<{
    keepAlive?: KeepAlivePattern | null;
}>();
</script>
