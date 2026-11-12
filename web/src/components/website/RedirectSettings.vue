<script setup lang="ts">
import { computed, ref } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { RedirectRule, Site } from '@/types';
import { request } from '@/api';
import { clone, useWebsiteSection } from './shared';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
import Status from '@/vendor/1panel/components/status/index.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const props = defineProps<{ site: Site }>();
const { busy, save, act, revision, resetRevision } = useWebsiteSection(() => props.site);
const rows = computed(() => props.site.redirects);
const draft = ref<RedirectRule>({
    id: '',
    name: '',
    enabled: true,
    type: 'domain',
    domains: [],
    path: '/',
    redirect: 301,
    target: 'https://',
    keepPath: true,
    redirectRoot: false,
  }),
  preview = ref<string>();
const form = ref<FormInstance>(),
  confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const open = ref(false);
const types = { domain: '域名', path: '路径', '404': '404' };
const available = computed(() =>
  props.site.domains.filter(
    (d) =>
      !rows.value.some(
        (r) => r.id !== draft.value?.id && r.type === 'domain' && r.domains.includes(d),
      ),
  ),
);
const rules: FormRules = { name: [{ required: true, message: '请输入名称', trigger: 'blur' }] };
function edit(row?: RedirectRule) {
  resetRevision();
  draft.value = row
    ? clone(row)
    : {
        id: '',
        name: '',
        enabled: true,
        type: 'domain',
        domains: [],
        path: '/',
        redirect: 301,
        target: 'https://',
        keepPath: true,
        redirectRoot: false,
      };
  open.value = true;
}
function changeType() {
  if (!draft.value) return;
  draft.value.name = draft.value.type === '404' ? '404' : '';
  draft.value.redirectRoot = false;
}
async function submit() {
  if (!(await form.value?.validate().catch(() => false))) return;
  if (await save('redirects', draft.value, 'POST')) open.value = false;
}
async function toggle(row: RedirectRule) {
  resetRevision();
  const expected = revision.value;
  if (
    await confirmRef.value?.ask({
      title: row.enabled ? '停止重定向' : '启用重定向',
      message: '确认修改此重定向规则的状态？',
      names: [row.name],
    })
  )
    await save('redirects', { ...row, enabled: !row.enabled }, 'POST', expected);
}
async function remove(row: RedirectRule) {
  resetRevision();
  const expected = revision.value;
  if (
    await confirmRef.value?.ask({
      title: '删除重定向',
      message: '删除后将不再执行此跳转。',
      names: [row.name],
    })
  )
    await save(`redirects/${row.id}`, undefined, 'DELETE', expected);
}
async function showPreview(row: RedirectRule) {
  await act(async () => {
    preview.value = (
      await request<{ config: string }>(`/sites/${props.site.id}/redirects/${row.id}/config`)
    ).config;
  }, '配置已生成');
}
const operations: FuTableOperationButton<RedirectRule>[] = [
  {
    label: '配置预览',
    click: (row) => {
      void showPreview(row);
    },
  },
  { label: '编辑', click: edit, disabled: (row) => !row.enabled },
  {
    label: '删除',
    type: 'danger',
    click: (row) => {
      void remove(row);
    },
  },
];
</script>
<template>
  <div class="section-toolbar">
    <el-button type="primary" plain :disabled="busy" @click="edit()">创建重定向</el-button>
  </div>
  <ComplexTable :data="rows"
    ><el-table-column label="名称" prop="name" min-width="120" card-type="name" /><el-table-column
      label="来源"
      min-width="160"
      card-type="content"
      ><template #default="{ row }">{{
        row.type === 'domain' ? row.domains.join(', ') : row.type === 'path' ? row.path : '404'
      }}</template></el-table-column
    ><el-table-column label="类型" width="80" card-type="content"
      ><template #default="{ row }">{{ types[row.type] }}</template></el-table-column
    ><el-table-column label="方式" prop="redirect" width="70" card-type="content" /><el-table-column
      label="目标 URL"
      min-width="200"
      show-overflow-tooltip
      card-type="content"
      ><template #default="{ row }">{{
        row.redirectRoot ? '/' : row.target
      }}</template></el-table-column
    ><el-table-column label="保留路径" width="100" card-type="content"
      ><template #default="{ row }">{{
        row.type === '404' ? '—' : row.keepPath ? '保留' : '不保留'
      }}</template></el-table-column
    ><el-table-column label="状态" width="120" card-type="status"
      ><template #default="{ row }"
        ><Status
          :status="row.enabled ? 'enable' : 'disable'"
          operate
          :disabled="busy"
          @click="toggle(row as RedirectRule)" /></template></el-table-column
    ><el-table-column label="操作" width="220" fixed="right" card-type="button"
      ><template #default="{ row }"
        ><TableOperationActions
          :buttons="operations"
          :row="row as RedirectRule"
          :ellipsis="3" /></template></el-table-column
  ></ComplexTable>
  <PanelDrawer
    v-if="open"
    :title="draft.id ? '编辑重定向' : '创建重定向'"
    :resource="draft.id ? draft.name : ''"
    @close="open = false"
    ><el-form ref="form" :model="draft" :rules="rules" label-position="top" @submit.prevent="submit"
      ><el-form-item label="名称" prop="name"
        ><el-input
          v-model="draft.name"
          :disabled="!!draft.id || draft.type === '404'"
          maxlength="128" /></el-form-item
      ><el-form-item label="类型"
        ><el-select v-model="draft.type" :disabled="!!draft.id" @change="changeType"
          ><el-option
            v-for="(label, value) in types"
            :key="value"
            :value="value"
            :label="label" /></el-select></el-form-item
      ><el-form-item label="重定向方式"
        ><el-select v-model="draft.redirect"
          ><el-option :value="301" label="301 永久重定向" /><el-option
            :value="302"
            label="302 临时重定向" /></el-select></el-form-item
      ><el-form-item v-if="draft.type === 'domain'" label="来源域名"
        ><el-select v-model="draft.domains" multiple
          ><el-option
            v-for="d in available"
            :key="d"
            :value="d"
            :label="d" /></el-select></el-form-item
      ><el-form-item v-if="draft.type === 'path'" label="来源路径"
        ><div class="field-control">
          <el-input v-model="draft.path" placeholder="/old/" /><span class="input-help"
            >按 ^路径(.*) 匹配。保留路径时将匹配后的剩余路径和查询参数附加到目标。</span
          >
        </div></el-form-item
      ><el-form-item v-if="draft.type === '404'" label="重定向到首页"
        ><el-switch v-model="draft.redirectRoot" /></el-form-item
      ><el-form-item v-if="!draft.redirectRoot" label="目标 URL"
        ><el-input v-model="draft.target" placeholder="https://example.com" /></el-form-item
      ><el-form-item v-if="draft.type !== '404'" label="保留路径"
        ><el-switch v-model="draft.keepPath" /></el-form-item></el-form
    ><template #footer
      ><el-button :disabled="busy" @click="open = false">取消</el-button
      ><el-button type="primary" :loading="busy" @click="submit">确认</el-button></template
    ></PanelDrawer
  >
  <PanelDrawer
    v-if="preview !== undefined"
    title="重定向配置预览"
    size="large"
    @close="preview = undefined"
    ><CodemirrorPro
      :model-value="preview"
      mode="nginx"
      readonly
      label="重定向配置"
      :height="450"
    /><template #footer
      ><el-button @click="preview = undefined">关闭</el-button></template
    ></PanelDrawer
  >
  <DeleteDialog ref="confirmRef" />
</template>
