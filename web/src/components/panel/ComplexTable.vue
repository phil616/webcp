<script setup lang="ts" generic="Row extends object">
import { computed, reactive, watch } from 'vue';
import OriginalTable from '@panel/components/table/complex/ComplexTable.vue';
import { usePanelUI } from '@/composables/usePanelUI';
const props = withDefaults(
  defineProps<{
    data: Row[];
    loading?: boolean;
    selection?: boolean;
    emptyText?: string;
    pageSize?: number;
    rowKey?: string | ((row: Row) => string);
  }>(),
  { selection: false, emptyText: '暂无数据', pageSize: 10, rowKey: 'id' },
);
const emit = defineEmits<{ selectionChange: [rows: Row[]] }>();
const pagination = reactive({ currentPage: 1, pageSize: props.pageSize, total: props.data.length });
const { isMobile } = usePanelUI();
const rows = computed(() =>
  props.data.slice(
    (pagination.currentPage - 1) * pagination.pageSize,
    pagination.currentPage * pagination.pageSize,
  ),
);
watch(
  () => props.data,
  () => {
    pagination.total = props.data.length;
    if (pagination.currentPage > Math.max(1, Math.ceil(props.data.length / pagination.pageSize)))
      pagination.currentPage = 1;
  },
);
watch(
  () => pagination.pageSize,
  () => {
    pagination.currentPage = 1;
  },
);
</script>
<template>
  <OriginalTable
    :data="rows"
    :loading="loading"
    :pagination-config="pagination"
    :cache-pagination="false"
    :row-key="rowKey"
    :empty-text="emptyText"
    :view-mode="isMobile ? 'card' : 'table'"
    @update:selects="(value) => emit('selectionChange', value)"
    ><template v-if="$slots.header" #header><slot name="header" /></template
    ><el-table-column v-if="selection" type="selection" width="42" /><slot /><template
      v-if="$slots.footerLeft"
      #footerLeft
      ><slot name="footerLeft" /></template
    ><template #empty
      ><slot name="empty"><el-empty :description="emptyText" :image-size="80" /></slot></template
  ></OriginalTable>
</template>
