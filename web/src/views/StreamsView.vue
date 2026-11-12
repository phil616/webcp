<script setup lang="ts">
import { computed, ref } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { StreamProxy } from '@/types';
import { request, useManager } from '@/api';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import TableSearch from '@/vendor/1panel/components/table/TableSearch.vue';
import TableRefresh from '@/vendor/1panel/components/table/TableRefresh.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import Status from '@/vendor/1panel/components/status/index.vue';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
const { state, busy, act, refresh } = useManager();
const search = ref(''),
  protocol = ref(''),
  revision = ref(0);
const draft = ref<Omit<StreamProxy, 'createdAt'>>();
const form = ref<FormInstance>();
const confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const items = computed(
  () =>
    state.value?.streams
      .filter(
        (item) =>
          (!protocol.value || item.protocol === protocol.value) &&
          `${item.name} ${item.listenAddress}:${item.listenPort} ${item.targetHost}:${item.targetPort}`
            .toLowerCase()
            .includes((search.value ?? '').toLowerCase()),
      )
      .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt)) ?? [],
);
const rules: FormRules = Object.fromEntries(
  ['name', 'listenAddress', 'listenPort', 'targetHost', 'targetPort'].map((key) => [
    key,
    [{ required: true, message: '请填写此项', trigger: 'blur' }],
  ]),
);
function address(host: string, port: number) {
  return `${host.includes(':') ? `[${host}]` : host}:${port}`;
}
function create() {
  revision.value = state.value!.revision;
  draft.value = {
    id: '',
    name: '',
    protocol: 'tcp',
    listenAddress: '0.0.0.0',
    listenPort: 9000,
    targetHost: '',
    targetPort: 9000,
    connectTimeout: 10,
    proxyTimeout: 600,
    udpResponses: 1,
    proxyProtocol: false,
    enabled: true,
  };
}
function edit(item: StreamProxy) {
  revision.value = state.value!.revision;
  draft.value = { ...item };
}
async function save() {
  if (!(await form.value?.validate().catch(() => false)) || !draft.value) return;
  const value = {
    ...draft.value,
    proxyProtocol: draft.value.protocol === 'tcp' && draft.value.proxyProtocol,
  };
  if (
    await act(() => request('/streams', 'POST', value, revision.value), '保存成功', {
      deploy: true,
    })
  )
    draft.value = undefined;
}
async function toggle(item: StreamProxy) {
  await act(() => request('/streams', 'POST', { ...item, enabled: !item.enabled }), '状态已更新', {
    deploy: true,
  });
}
async function remove(item: StreamProxy) {
  if (
    await confirmRef.value?.ask({
      title: '删除四层代理',
      message: '删除后，该监听地址将停止转发。端口发布变化会重建容器并短暂中断连接。',
      names: [item.name],
    })
  )
    await act(() => request(`/streams/${item.id}`, 'DELETE'), '删除成功', { deploy: true });
}
const operations: FuTableOperationButton<StreamProxy>[] = [
  { label: '编辑', click: edit },
  {
    label: '删除',
    type: 'danger',
    click: (item) => {
      void remove(item);
    },
  },
];
</script>
<template>
  <div class="route-view">
    <LayoutContent>
      <template #prompt
        ><el-alert
          type="info"
          :closable="false"
          show-icon
          title="TCP / UDP 单目标转发，TCP 可透传 TLS。增删、启停或修改监听配置会重建托管容器，短暂中断网站和代理连接；仅修改目标和超时使用重载。"
      /></template>
      <template #leftToolBar
        ><el-button type="primary" @click="create">创建四层代理</el-button></template
      >
      <template #rightToolBar>
        <el-select v-model="protocol" class="filter-select" aria-label="代理协议筛选"
          ><template #prefix>协议</template><el-option value="" label="全部" /><el-option
            value="tcp"
            label="TCP" /><el-option value="udp" label="UDP"
        /></el-select>
        <TableSearch v-model:search-name="search" placeholder="搜索名称或地址" /><TableRefresh
          :disabled="busy"
          aria-label="刷新四层代理"
          @search="refresh"
        />
      </template>
      <template #main
        ><ComplexTable :data="items">
          <el-table-column label="名称" card-type="name" min-width="160"
            ><template #default="{ row }"
              ><el-button type="primary" link @click="edit(row as StreamProxy)">{{
                row.name
              }}</el-button></template
            ></el-table-column
          >
          <el-table-column label="协议" width="90" card-type="content"
            ><template #default="{ row }"
              ><el-tag effect="plain">{{ row.protocol.toUpperCase() }}</el-tag></template
            ></el-table-column
          >
          <el-table-column label="监听地址" min-width="190" card-type="content"
            ><template #default="{ row }"
              ><span class="mono">{{ address(row.listenAddress, row.listenPort) }}</span></template
            ></el-table-column
          >
          <el-table-column label="目标地址" min-width="220" card-type="content"
            ><template #default="{ row }"
              ><span class="mono">{{ address(row.targetHost, row.targetPort) }}</span></template
            ></el-table-column
          >
          <el-table-column label="状态" card-type="status" width="130"
            ><template #default="{ row }"
              ><Status
                :status="row.enabled ? 'enable' : 'disable'"
                :operate="!busy"
                @click="!busy && toggle(row as StreamProxy)" /></template
          ></el-table-column>
          <el-table-column label="会话超时" width="110" card-type="content"
            ><template #default="{ row }">{{ row.proxyTimeout }} 秒</template></el-table-column
          >
          <el-table-column label="操作" card-type="button" width="140" fixed="right"
            ><template #default="{ row }"
              ><TableOperationActions :buttons="operations" :row="row as StreamProxy" /></template
          ></el-table-column> </ComplexTable
      ></template>
    </LayoutContent>
    <PanelDrawer
      v-if="draft"
      :title="draft.id ? '编辑四层代理' : '创建四层代理'"
      :resource="draft.id ? draft.name : ''"
      @close="draft = undefined"
    >
      <el-form ref="form" :model="draft" :rules="rules" label-position="top" @submit.prevent="save">
        <el-form-item label="名称" prop="name"
          ><el-input v-model="draft.name" maxlength="128" placeholder="例如数据库转发"
        /></el-form-item>
        <el-form-item label="协议"
          ><el-radio-group v-model="draft.protocol"
            ><el-radio-button value="tcp">TCP</el-radio-button
            ><el-radio-button value="udp">UDP</el-radio-button></el-radio-group
          ></el-form-item
        >
        <el-form-item label="监听地址" prop="listenAddress"
          ><div class="field-control">
            <el-input v-model="draft.listenAddress" placeholder="0.0.0.0 或指定宿主机 IP" /><span
              class="input-help"
              >宿主机 IP，支持 IPv4 / IPv6；0.0.0.0 表示所有 IPv4 地址。</span
            >
          </div></el-form-item
        >
        <el-form-item label="监听端口" prop="listenPort"
          ><el-input-number v-model="draft.listenPort" :min="1" :max="65535"
        /></el-form-item>
        <el-form-item label="目标地址" prop="targetHost"
          ><div class="field-control">
            <el-input v-model="draft.targetHost" placeholder="域名或 IP，不含协议和端口" /><span
              class="input-help"
              >127.0.0.1 指向容器自身。宿主机服务可使用
              host.docker.internal，服务需监听容器可访问的地址。IPv6
              直接填写地址，不加方括号。</span
            >
          </div></el-form-item
        >
        <el-form-item label="目标端口" prop="targetPort"
          ><el-input-number v-model="draft.targetPort" :min="1" :max="65535"
        /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="draft.enabled" /></el-form-item>
        <el-collapse
          ><el-collapse-item title="高级设置" name="advanced">
            <el-form-item label="连接超时（秒）"
              ><el-input-number v-model="draft.connectTimeout" :min="1" :max="3600"
            /></el-form-item>
            <el-form-item label="会话空闲超时（秒）"
              ><el-input-number v-model="draft.proxyTimeout" :min="1" :max="86400"
            /></el-form-item>
            <el-form-item v-if="draft.protocol === 'udp'" label="每个 UDP 请求预期响应数"
              ><div class="field-control">
                <el-input-number v-model="draft.udpResponses" :min="0" :max="1000" /><span
                  class="input-help"
                  >用于判断 UDP 会话完成；0 表示不等待响应。</span
                >
              </div></el-form-item
            >
            <el-form-item v-else label="发送 PROXY protocol"
              ><div class="field-control">
                <el-switch v-model="draft.proxyProtocol" /><span class="input-help"
                  >向目标发送客户端来源信息，仅在目标服务支持 PROXY protocol 时开启。</span
                >
              </div></el-form-item
            >
          </el-collapse-item></el-collapse
        >
      </el-form>
      <template #footer
        ><el-button :disabled="busy" @click="draft = undefined">取消</el-button
        ><el-button type="primary" :loading="busy" @click="save">保存</el-button></template
      >
    </PanelDrawer>
    <DeleteDialog ref="confirmRef" />
  </div>
</template>
