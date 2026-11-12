<script setup lang="ts">
import type { Site } from '@/types';
import NginxConfig from '@panel/views/website/website/config/resource/nginx/index.vue';
defineProps<{ site: Site }>();
</script>
<template>
  <NginxConfig :id="site.id" />
</template>
