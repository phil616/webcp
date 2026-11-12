<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { DomainBinding, Site } from '@/types';
import { useManager } from '@/api';
import { clone, lines, useWebsiteSection } from './shared';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
const props = defineProps<{ site: Site }>();
const { state, busy, save, resetRevision } = useWebsiteSection(() => props.site);
const { refresh } = useManager();
const open = ref(false),
  input = ref(''),
  port = ref(80),
  ssl = ref(false);
const pending = ref<DomainBinding[]>([]);
const confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const rows = computed(() => props.site.bindings);
function create() {
  resetRevision();
  pending.value = clone(rows.value);
  input.value = '';
  port.value = 80;
  ssl.value = false;
  open.value = true;
}
watch(port, (value) => {
  if (value === 80) ssl.value = false;
  if (value === 443) ssl.value = true;
});
async function add() {
  const values = lines(input.value).map((domain) => ({
    domain: domain.toLowerCase(),
    port: port.value,
    ssl: port.value === 443 || (port.value !== 80 && ssl.value),
  }));
  if (await save('domains', { bindings: [...pending.value, ...values] })) open.value = false;
}
async function toggle(row: DomainBinding) {
  resetRevision();
  await save('domains', {
    bindings: rows.value.map((value) => (value === row ? { ...value, ssl: !value.ssl } : value)),
  });
}
async function remove(row: DomainBinding) {
  const expected = state.value!.revision;
  if (
    await confirmRef.value?.ask({
      title: '删除域名',
      message: '删除后，此域名或端口将从网站配置移除。网站至少保留一条绑定。',
      names: [`${row.domain}:${row.port}`],
    })
  )
    await save(
      'domains',
      {
        bindings: rows.value.filter(
          (value) => value.domain !== row.domain || value.port !== row.port,
        ),
      },
      'PUT',
      expected,
    );
}
const operations: FuTableOperationButton<DomainBinding>[] = [
  {
    label: '删除',
    type: 'danger',
    disabled: () => rows.value.length === 1,
    click: (row) => {
      void remove(row);
    },
  },
];
</script>
<template>
  <div class="section-toolbar">
    <el-button type="primary" plain :disabled="busy" @click="create">添加域名</el-button
    ><el-button :disabled="busy" @click="refresh">刷新</el-button>
  </div>
  <ComplexTable :data="rows" :row-key="(row) => `${row.domain}:${row.port}`"
    ><el-table-column label="域名" prop="domain" min-width="220" card-type="name" /><el-table-column
      label="端口"
      prop="port"
      width="100"
      card-type="content" /><el-table-column label="SSL" width="100" card-type="status"
      ><template #default="{ row }"
        ><el-switch
          :model-value="row.ssl"
          :disabled="busy || row.port === 80 || row.port === 443"
          @change="toggle(row as DomainBinding)" /></template></el-table-column
    ><el-table-column label="操作" width="100" card-type="button"
      ><template #default="{ row }"
        ><TableOperationActions
          :buttons="operations"
          :row="row as DomainBinding" /></template></el-table-column
  ></ComplexTable>
  <p class="input-help">
    绑定端口在网站内共享，SSL 在启用 HTTPS 后生效。启用 HTTPS 时同时监听默认 HTTPS
    端口。自定义端口变化会重建容器并短暂中断连接。
  </p>
  <PanelDrawer v-if="open" title="添加域名" @close="open = false"
    ><el-form label-position="top" @submit.prevent="add"
      ><el-form-item label="域名 / IP"
        ><el-input
          v-model="input"
          type="textarea"
          :rows="6"
          placeholder="每行一个域名或 IP，不含协议和端口" /></el-form-item
      ><el-form-item label="端口"
        ><el-input-number v-model="port" :min="1" :max="65535" /></el-form-item
      ><el-form-item label="SSL"
        ><el-switch v-model="ssl" :disabled="port === 80 || port === 443" /><span class="input-help"
          >80 为 HTTP，443 为 HTTPS；其他端口可选择 SSL。</span
        ></el-form-item
      ></el-form
    ><template #footer
      ><el-button @click="open = false">取消</el-button
      ><el-button type="primary" :loading="busy" :disabled="!input.trim()" @click="add"
        >确认</el-button
      ></template
    ></PanelDrawer
  >
  <DeleteDialog ref="confirmRef" />
</template>
