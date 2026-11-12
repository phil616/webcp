<script setup lang="ts">
import type { Site } from '@/types';
import { OfficialHTTPS } from '@panel/entry';
defineProps<{ site: Site }>();
</script>
<template><OfficialHTTPS :key="site.id" :id="site.panelId" /></template>
