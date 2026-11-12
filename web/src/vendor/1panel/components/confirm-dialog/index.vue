<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/confirm-dialog/index.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <DialogPro v-model="submitVisible" :title="header" size="mini" @close="onCancel">
    <div>
      <span v-if="operationInfo" class="operation-info">{{ operationInfo }}</span>
      <slot />
      <div :style="{ 'margin-top': operationInfo ? '10px' : '0px' }">
        <span style="font-size: 12px">{{ $t('commons.msg.operateConfirm') }}</span>
        <span style="font-size: 12px; color: red; font-weight: 500">'{{ submitInputInfo }}'</span>
      </div>
      <el-input aria-label="确认内容" style="margin-top: 10px" v-model="submitInput"></el-input>
    </div>
    <template #footer>
      <span class="dialog-footer">
        <el-button @click="onCancel">
          {{ $t('commons.button.cancel') }}
        </el-button>
        <el-button type="primary" :disabled="submitInput !== submitInputInfo" @click="onConfirm">
          {{ $t('commons.button.confirm') }}
        </el-button>
      </span>
    </template>
  </DialogPro>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import DialogPro from '../dialog-pro/index.vue';

const header = ref();
const operationInfo = ref();
const submitInputInfo = ref();
const submitVisible = ref(false);

const submitInput = ref();

interface DialogProps {
  header: string;
  operationInfo: string;
  submitInputInfo: string;
}

const acceptParams = (props: DialogProps): void => {
  submitVisible.value = true;
  header.value = props.header;
  operationInfo.value = props.operationInfo;
  submitInputInfo.value = props.submitInputInfo;
  submitInput.value = '';
};
const emit = defineEmits(['confirm', 'cancel']);

const onConfirm = async () => {
  emit('confirm');
  submitVisible.value = false;
};

const onCancel = async () => {
  emit('cancel');
  submitVisible.value = false;
};

defineExpose({
  acceptParams,
});
</script>

<style scoped>
.operation-info {
  white-space: pre-line;
  font-size: 12px;
  line-height: 20px;
}
</style>
