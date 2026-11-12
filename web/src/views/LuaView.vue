<script setup lang="ts">
import { computed, ref } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { Lua } from '@/types';
import { useManager, request, notify } from '@/api';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import TableSearch from '@/vendor/1panel/components/table/TableSearch.vue';
import TableRefresh from '@/vendor/1panel/components/table/TableRefresh.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
const { state, busy, act, refresh } = useManager();
const draft = ref<Lua>();
const search = ref(''),
  kind = ref(''),
  revision = ref(0);
const form = ref<FormInstance>(),
  confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const items = computed(
  () =>
    state.value?.lua
      .filter(
        (item) =>
          (!kind.value || item.kind === kind.value) &&
          `${item.name} ${item.description}`
            .toLowerCase()
            .includes((search.value ?? '').toLowerCase()),
      )
      .reverse() ?? [],
);
const rules: FormRules = {
  name: [
    { required: true, message: '请输入资源名称', trigger: 'blur' },
    {
      pattern: /^[A-Za-z_][A-Za-z0-9_]{0,63}$/,
      message: '使用英文字母、数字、下划线，以字母或下划线开头',
      trigger: 'blur',
    },
  ],
  code: [{ required: true, message: '请输入 Lua 代码', trigger: 'blur' }],
};
function create(type: 'module' | 'block') {
  revision.value = state.value!.revision;
  draft.value = {
    id: '',
    name: '',
    kind: type,
    description: '',
    code:
      type === 'module'
        ? 'local M = {}\n\nfunction M.hello()\n    return "Hello from OpenResty"\nend\n\nreturn M\n'
        : '-- 适用于 access 阶段\nngx.header["X-Powered-By"] = "Resty"\n',
  };
}
function edit(item: Lua) {
  revision.value = state.value!.revision;
  draft.value = { ...item };
}
async function save() {
  if (!(await form.value?.validate().catch(() => false))) return;
  if (
    await act(() => request('/lua', 'POST', draft.value, revision.value), '保存成功', {
      deploy: true,
    })
  )
    draft.value = undefined;
}
function usage(item: Lua) {
  return item.kind === 'block'
    ? `${state.value?.sites.filter((site) => Object.values(site.luaBindings ?? {}).includes(item.id)).length ?? 0} 个网站`
    : `require("${item.name}")`;
}
async function copy(item: Lua) {
  try {
    await navigator.clipboard.writeText(
      item.kind === 'module' ? `local ${item.name} = require("${item.name}")` : item.code,
    );
    notify('复制成功');
  } catch {
    notify('剪贴板不可用，请在编辑器中手动复制。', true);
  }
}
async function remove(item: Lua) {
  if (
    await confirmRef.value?.ask({
      title: '删除 Lua 资源',
      message:
        item.kind === 'module'
          ? '请确认没有代码块继续 require 此模块；删除后，相关请求逻辑将无法加载模块。'
          : '被网站绑定的代码块不能删除，请先解除绑定。',
      names: [item.name],
    })
  )
    await act(() => request(`/lua/${item.id}`, 'DELETE'), '删除成功', { deploy: true });
}
const operations: FuTableOperationButton<Lua>[] = [
  { label: '编辑', click: edit },
  {
    label: '复制调用',
    click: (item) => {
      void copy(item);
    },
  },
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
    <LayoutContent
      ><template #leftToolBar
        ><el-button type="primary" @click="create('module')">创建模块</el-button
        ><el-button type="primary" plain @click="create('block')">创建代码块</el-button></template
      ><template #rightToolBar
        ><el-select v-model="kind" class="filter-select" aria-label="Lua 类型筛选"
          ><template #prefix>类型</template><el-option value="" label="全部" /><el-option
            value="module"
            label="模块" /><el-option value="block" label="代码块" /></el-select
        ><TableSearch v-model:search-name="search" placeholder="搜索名称或描述" /><TableRefresh
          aria-label="刷新 Lua 资源"
          :disabled="busy"
          @search="refresh" /></template
      ><template #main
        ><ComplexTable :data="items"
          ><el-table-column label="名称" card-type="name" min-width="180"
            ><template #default="{ row }"
              ><el-button type="primary" link @click="edit(row as Lua)">{{
                row.name
              }}</el-button></template
            ></el-table-column
          ><el-table-column label="类型" width="120" card-type="content"
            ><template #default="{ row }"
              ><el-tag effect="plain" :type="row.kind === 'module' ? 'primary' : 'success'">{{
                row.kind === 'module' ? '模块' : '代码块'
              }}</el-tag></template
            ></el-table-column
          ><el-table-column
            label="描述"
            prop="description"
            min-width="220"
            show-overflow-tooltip
            card-type="content" /><el-table-column
            label="使用方式"
            min-width="220"
            card-type="content"
            ><template #default="{ row }"
              ><span class="mono">{{ usage(row as Lua) }}</span></template
            ></el-table-column
          ><el-table-column label="操作" card-type="button" width="200" fixed="right"
            ><template #default="{ row }"
              ><TableOperationActions
                :buttons="operations"
                :row="row as Lua"
                :ellipsis="2"
                trigger="click" /></template></el-table-column></ComplexTable></template></LayoutContent
    ><PanelDrawer
      v-if="draft"
      :title="draft.id ? '编辑 Lua 资源' : draft.kind === 'module' ? '创建模块' : '创建代码块'"
      :resource="draft.id ? draft.name : ''"
      size="large"
      @close="draft = undefined"
      ><el-form ref="form" :model="draft" :rules="rules" label-position="top" @submit.prevent="save"
        ><el-form-item label="资源名称" prop="name"
          ><div class="field-control">
            <el-input v-model="draft.name" maxlength="64" placeholder="例如 response_utils" /><span
              class="input-help"
              >名称全局唯一。模块改名后，需要同步调整代码块中的 require 引用。</span
            >
          </div></el-form-item
        ><el-form-item label="资源类型"
          ><el-radio-group v-model="draft.kind" :disabled="!!draft.id"
            ><el-radio-button value="module">模块</el-radio-button
            ><el-radio-button value="block">代码块</el-radio-button></el-radio-group
          ></el-form-item
        ><el-form-item label="描述"
          ><el-input
            v-model="draft.description"
            maxlength="500"
            placeholder="用途、参数或适用阶段" /></el-form-item
        ><el-form-item label="Lua 代码" prop="code"
          ><CodemirrorPro
            v-model="draft.code"
            mode="lua"
            label="Lua 代码"
            :height="380" /></el-form-item
        ><el-alert
          :closable="false"
          type="info"
          show-icon
          title="代码块在网站设置中绑定阶段。应用配置前检查 Lua 语法；依赖和业务行为需通过实际请求验证。" /></el-form
      ><template #footer
        ><el-button :disabled="busy" @click="draft = undefined">取消</el-button
        ><el-button type="primary" :loading="busy" @click="save">保存</el-button></template
      ></PanelDrawer
    ><DeleteDialog ref="confirmRef" />
  </div>
</template>
