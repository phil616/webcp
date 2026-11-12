<!-- SPDX-License-Identifier: GPL-3.0-only
Derived from 1Panel (frontend/src/components/table/TableSearch.vue), commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
Copyright belongs to the upstream 1Panel contributors. Local adaptations: see manifest.json.
-->
<template>
  <div>
    <div class="search-button">
      <el-input
        clearable
        v-model="searchInfo"
        @clear="search()"
        @keydown.enter.stop.prevent="search()"
        @blur="searchIfChanged()"
        :disabled="props.disabled"
        :placeholder="props.placeholder || $t('commons.button.search')"
      >
        <template #suffix>
          <button
            type="button"
            class="search-submit"
            :disabled="props.disabled"
            :aria-label="$t('commons.button.search')"
            @mousedown.prevent
            @click="search()"
          >
            <el-icon><Search /></el-icon>
          </button>
        </template>
      </el-input>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onActivated, ref, watch } from 'vue';
import { Search } from '@element-plus/icons-vue';
defineOptions({ name: 'TableSearch' });

const emit = defineEmits(['search', 'update:searchName']);
const searchInfo = ref();
let lastSearch: string | number | undefined;
const props = defineProps({
  placeholder: String,
  disabled: {
    type: Boolean,
    default: false,
  },
  searchName: {
    type: [String, Number],
    default: undefined,
  },
});

watch(
  () => props.searchName,
  (newVal) => {
    if (searchInfo.value !== newVal) {
      searchInfo.value = newVal;
      lastSearch = newVal;
    }
  },
  { immediate: true },
);

onActivated(() => {
  searchInfo.value = props.searchName;
});

const search = () => {
  if (props.disabled) return;
  lastSearch = searchInfo.value;
  emit('update:searchName', searchInfo.value);
  emit('search');
};

const searchIfChanged = () => {
  if (searchInfo.value !== lastSearch) search();
};
</script>

<style scoped>
.search-submit {
  display: inline-flex;
  align-items: center;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.search-submit:disabled {
  cursor: not-allowed;
}
</style>
