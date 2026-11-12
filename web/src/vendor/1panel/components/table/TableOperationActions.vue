<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/table/TableOperationActions.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <div class="fu-table-operations" @click.stop>
    <span
      v-for="(button, index) in actions.inline"
      :key="getButtonKey(button, index)"
      class="fu-table-operations__action"
    >
      <el-button
        class="fu-table-operations__button"
        link
        :type="button.type || 'primary'"
        :disabled="isOperationDisabled(button, row)"
        @click.stop="handleButtonClick(button)"
      >
        <el-icon v-if="button.icon" class="mr-1"><component :is="button.icon" /></el-icon>
        <el-badge is-dot :hidden="!hasDot(button)">
          <span>{{ button.label }}</span>
        </el-badge>
      </el-button>
    </span>
    <span v-if="actions.more.length" class="fu-table-operations__action">
      <el-dropdown class="fu-table-more-button" :trigger="trigger" @command="handleButtonClick">
        <span class="fu-table-operations__dropdown-trigger">
          <el-button class="fu-table-operations__button" link type="primary">
            <el-badge is-dot :hidden="!actions.more.some(hasDot)">{{
              t('fu.table.more')
            }}</el-badge>
          </el-button>
        </span>
        <template #dropdown>
          <el-dropdown-menu :style="dropdownStyle">
            <el-dropdown-item
              v-for="(button, index) in actions.more"
              :key="getButtonKey(button, index)"
              :command="button"
              :disabled="isOperationDisabled(button, row)"
              :divided="button.divided"
            >
              <el-icon v-if="button.icon" class="mr-1"><component :is="button.icon" /></el-icon>
              <el-badge is-dot :hidden="!hasDot(button)">
                <span>{{ button.label }}</span>
              </el-badge>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </span>
  </div>
</template>

<script setup lang="ts" generic="Row extends object">
import { computed, type PropType } from 'vue';
import { useI18n } from 'vue-i18n';

import { isOperationDisabled, isOperationVisible, type FuTableOperationButton } from './shared';

defineOptions({ name: 'FuTableOperationActions' });

type DropdownTrigger = 'hover' | 'click' | 'contextmenu';
type DropdownTriggerValue = DropdownTrigger | DropdownTrigger[];

const { t } = useI18n();
const props = defineProps({
  buttons: { type: Array as PropType<FuTableOperationButton<Row>[]>, default: () => [] },
  row: { type: Object as PropType<Row>, required: true },
  ellipsis: { type: Number, default: 2 },
  extra: { type: Number, default: 0 },
  trigger: {
    type: [String, Array] as PropType<DropdownTriggerValue>,
    default: 'hover',
  },
  dropdownStyle: {
    type: Object as PropType<Record<string, string> | undefined>,
    default: undefined,
  },
});

const actions = computed(() => {
  const visible = props.buttons.filter((button) => isOperationVisible(button, props.row));
  const limit = Math.max(props.ellipsis + props.extra, 0);
  return {
    inline: limit > 0 ? visible.slice(0, limit) : [],
    more: visible.slice(limit),
  };
});

const hasDot = (button: FuTableOperationButton<Row>) =>
  typeof button.dot === 'function' ? button.dot(props.row) : Boolean(button.dot);

const handleButtonClick = (button: FuTableOperationButton<Row>) => {
  if (!isOperationDisabled(button, props.row)) {
    button.click?.(props.row);
  }
};

const getButtonKey = (button: FuTableOperationButton<Row>, index: number) =>
  button.key ??
  `${button.label || button.command || button.type || (button.icon as { name?: string })?.name || 'button'}-${index}`;
</script>
