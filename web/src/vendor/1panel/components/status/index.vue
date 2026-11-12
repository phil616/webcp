<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/status/index.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <span class="status-wrapper" :class="{ 'is-disabled': isDisabled }" :aria-disabled="isDisabled">
    <el-tooltip v-if="msg && msg != ''" effect="dark" v-bind="TABLE_TOOLTIP_OPTIONS">
      <template #content>
        <div class="content">{{ msg }}</div>
      </template>
      <el-tag size="small" :type="getType(statusItem)" round effect="light">
        <span class="flx-align-center">
          <span v-if="statusItem != ''">{{ $t('commons.status.' + statusItem) }}</span>
          <el-icon v-if="loadingIcon(statusItem)" class="is-loading">
            <Loading />
          </el-icon>
        </span>
      </el-tag>
    </el-tooltip>
    <template v-else>
      <el-tag size="small" :type="getType(statusItem)" round effect="light" v-if="!operate">
        <span class="flx-align-center">
          <span v-if="statusItem != ''">{{ $t('commons.status.' + statusItem) }}</span>
          <el-icon v-if="loadingIcon(statusItem)" class="is-loading">
            <Loading />
          </el-icon>
        </span>
      </el-tag>
      <el-button size="small" v-else :type="getType(statusItem)" plain round :disabled="isDisabled">
        <span v-if="statusItem != ''">{{ $t('commons.status.' + statusItem) }}</span>
        <el-icon v-if="loadingIcon(statusItem)" class="is-loading">
          <Loading />
        </el-icon>
        <el-icon v-if="stopIcon(statusItem)"><VideoPause /></el-icon>
        <el-icon v-if="runningIcon(statusItem)"><VideoPlay /></el-icon>
      </el-button>
    </template>
  </span>
</template>

<script lang="ts" setup>
import { TABLE_TOOLTIP_OPTIONS } from './../table/shared';
import { computed } from 'vue';
import { Loading, VideoPause, VideoPlay } from '@element-plus/icons-vue';

const props = defineProps({
  status: String,
  msg: String,
  hasIcon: Boolean,
  disabled: Boolean,
  operate: {
    type: Boolean,
    default: false,
    required: false,
  },
});

const statusItem = computed(() => {
  return props.status?.toLowerCase() || '';
});

const isDisabled = computed(() => Boolean(props.disabled));

const getType = (status: string) => {
  if (status.includes('error') || status.includes('err')) {
    return 'danger';
  }
  switch (status) {
    case 'running':
    case 'free':
    case 'success':
    case 'enable':
    case 'done':
    case 'healthy':
    case 'unused':
    case 'executing':
    case 'new':
      return 'success';
    case 'stopped':
    case 'exceptional':
    case 'disable':
    case 'unhealthy':
    case 'failed':
    case 'crashed':
    case 'lost':
    case 'exited':
    case 'notfound':
    case 'inaccessible':
      return 'danger';
    case 'paused':
    case 'blocked':
    case 'shutdown':
    case 'pmsuspended':
    case 'dead':
    case 'removing':
    case 'deleted':
    case 'conflict':
    case 'partial':
    case 'degraded':
      return 'warning';
    case 'duplicate':
    case 'unexecuted':
    case 'canceled':
    case 'inactive':
    case 'unknown':
      return 'info';
    default:
      return 'primary';
  }
};

const loadingStatus = [
  'installing',
  'building',
  'restarting',
  'upgrading',
  'rebuilding',
  'recreating',
  'creating',
  'starting',
  'shutdown',
  'removing',
  'applying',
  'uninstalling',
  'downloading',
  'packing',
  'sending',
  'waiting',
  'executing',
  'loading',
];

const stopStatus = ['stopped', 'exited', 'disable'];
const runningStatus = ['running', 'enable'];

const loadingIcon = (status: string): boolean => {
  return loadingStatus.indexOf(status) > -1;
};
const stopIcon = (status: string): boolean => {
  return stopStatus.indexOf(status.toLocaleLowerCase()) > -1;
};
const runningIcon = (status: string): boolean => {
  return runningStatus.indexOf(status.toLocaleLowerCase()) > -1;
};
</script>

<style lang="scss" scoped>
.status-wrapper {
  display: inline-flex;
}

.status-wrapper.is-disabled {
  cursor: not-allowed;
}

.status-wrapper.is-disabled :deep(.el-tag) {
  opacity: 0.55;
  filter: grayscale(1);
}

.content {
  width: 300px;
  word-break: break-all;
}

.status-icon {
  width: 1em;
  height: 1em;
}
</style>
