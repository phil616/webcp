<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { ProxyRule, Site } from '@/types';
import { request } from '@/api';
import { clone, defaultProxy, useWebsiteSection } from './shared';
import CORSFields from './CORSFields.vue';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
import Status from '@/vendor/1panel/components/status/index.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const props = defineProps<{ site: Site }>();
const { busy, save, act, revision, resetRevision } = useWebsiteSection(() => props.site);
const rows = computed(() => props.site.proxies);
const draft = ref<ProxyRule>(defaultProxy()),
  cache = ref<Site['proxyCache']>(),
  preview = ref<string>();
const form = ref<FormInstance>(),
  confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const open = ref(false);
const tab = ref('basic'),
  sniTouched = ref(false);
const rules: FormRules = Object.fromEntries(
  ['name', 'match', 'proxyPass', 'proxyHost'].map((k) => [
    k,
    [{ required: true, message: '请填写此项', trigger: 'blur' }],
  ]),
);
const units = [
  { value: 's', label: '秒' },
  { value: 'm', label: '分钟' },
  { value: 'h', label: '小时' },
  { value: 'd', label: '天' },
];
function edit(row?: ProxyRule) {
  resetRevision();
  draft.value = row ? clone(row) : defaultProxy();
  tab.value = 'basic';
  sniTouched.value = false;
  open.value = true;
}
watch(
  () => draft.value?.proxyPass,
  (value) => {
    if (draft.value && !draft.value.id && !sniTouched.value)
      draft.value.sni = value?.startsWith('https://') ?? false;
  },
);
function autoHost() {
  try {
    const value = new URL(draft.value.proxyPass);
    draft.value.proxyHost =
      /[a-z]/i.test(value.hostname) && value.hostname.includes('.') && !value.hostname.includes(':')
        ? value.host
        : '$host';
  } catch {
    /* The form/API reports invalid URLs on submit. */
  }
}
async function submit() {
  if (!(await form.value?.validate().catch(() => false))) return;
  if (await save('proxies', draft.value, 'POST')) open.value = false;
}
async function toggle(row: ProxyRule) {
  resetRevision();
  const version = revision.value;
  if (
    await confirmRef.value?.ask({
      title: row.enabled ? '停止反向代理' : '启用反向代理',
      message: row.enabled ? '停止后，此规则将不再转发请求。' : '确认启用此代理规则？',
      names: [row.name],
    })
  )
    await save('proxies', { ...row, enabled: !row.enabled }, 'POST', version);
}
async function remove(row: ProxyRule) {
  resetRevision();
  const expected = revision.value;
  if (
    await confirmRef.value?.ask({
      title: '删除反向代理',
      message: '删除此规则后，匹配路径将恢复网站默认处理。',
      names: [row.name],
    })
  )
    await save(`proxies/${row.id}`, undefined, 'DELETE', expected);
}
async function showPreview(row: ProxyRule) {
  await act(async () => {
    preview.value = (
      await request<{ config: string }>(`/sites/${props.site.id}/proxies/${row.id}/config`)
    ).config;
  }, '配置已生成');
}
function cacheSettings() {
  resetRevision();
  cache.value = clone(props.site.proxyCache);
}
async function clear() {
  if (
    await confirmRef.value?.ask({
      title: '清理代理缓存',
      message: '立即清理当前网站的磁盘代理缓存，不影响其他网站。',
      names: [props.site.name],
    })
  )
    await act(() => request(`/sites/${props.site.id}/proxy-cache/clear`, 'POST'), '缓存已清理');
}
const operations: FuTableOperationButton<ProxyRule>[] = [
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
    <el-button type="primary" plain :disabled="busy" @click="edit()">创建反向代理</el-button
    ><el-button :disabled="busy" @click="cacheSettings">缓存设置</el-button
    ><el-button type="primary" link :disabled="busy" @click="clear">清理代理缓存</el-button>
  </div>
  <ComplexTable :data="rows"
    ><el-table-column label="名称" prop="name" min-width="130" card-type="name" /><el-table-column
      label="代理目录"
      min-width="130"
      card-type="content"
      ><template #default="{ row }"
        ><span class="mono">{{ row.modifier }} {{ row.match }}</span></template
      ></el-table-column
    ><el-table-column
      label="代理地址"
      prop="proxyPass"
      min-width="220"
      show-overflow-tooltip
      card-type="content" /><el-table-column label="缓存" min-width="140" card-type="content"
      ><template #default="{ row }"
        ><el-tag :type="row.cache ? 'success' : 'info'"
          >服务端：{{ row.cache ? `${row.serverCacheTime}${row.serverCacheUnit}` : '关闭' }}</el-tag
        ><el-tag v-if="row.cacheTime !== 0" class="mt-1" effect="plain"
          >浏览器：{{ row.cacheTime < 0 ? '不缓存' : `${row.cacheTime}${row.cacheUnit}` }}</el-tag
        ></template
      ></el-table-column
    ><el-table-column label="状态" width="120" card-type="status"
      ><template #default="{ row }"
        ><Status
          :status="row.enabled ? 'enable' : 'disable'"
          operate
          :disabled="busy"
          @click="toggle(row as ProxyRule)" /></template></el-table-column
    ><el-table-column label="操作" width="220" fixed="right" card-type="button"
      ><template #default="{ row }"
        ><TableOperationActions
          :buttons="operations"
          :row="row as ProxyRule"
          :ellipsis="3" /></template></el-table-column
  ></ComplexTable>
  <PanelDrawer
    v-if="open"
    :title="draft.id ? '编辑反向代理' : '创建反向代理'"
    :resource="draft.id ? draft.name : ''"
    size="large"
    @close="open = false"
  >
    <el-form ref="form" :model="draft" :rules="rules" label-position="top" @submit.prevent="submit"
      ><el-tabs v-model="tab" type="border-card"
        ><el-tab-pane label="代理设置" name="basic">
          <el-form-item label="名称" prop="name"
            ><el-input v-model="draft.name" :disabled="!!draft.id" maxlength="128"
          /></el-form-item>
          <el-form-item label="匹配规则"
            ><el-select v-model="draft.modifier"
              ><el-option value="" label="前缀匹配" /><el-option
                value="="
                label="= 精确匹配" /><el-option value="^~" label="^~ 优先前缀匹配" /><el-option
                value="~"
                label="~ 正则匹配" /><el-option
                value="~*"
                label="~* 正则匹配（不区分大小写）" /></el-select
          ></el-form-item>
          <el-form-item label="代理目录" prop="match"
            ><el-input v-model="draft.match" placeholder="/api/"
          /></el-form-item>
          <el-form-item label="代理地址" prop="proxyPass"
            ><div class="field-control">
              <el-input
                v-model="draft.proxyPass"
                @blur="autoHost"
                placeholder="http://127.0.0.1:8080"
              /><span class="input-help"
                >单个 HTTP(S) 目标。URL 带 URI 时替换匹配的目录；不带 URI 时保留请求路径。127.0.0.1
                指向 OpenResty 容器，宿主机使用 host.docker.internal。</span
              >
            </div></el-form-item
          >
          <el-form-item label="发送域名" prop="proxyHost"
            ><el-input v-model="draft.proxyHost" placeholder="$host"
          /></el-form-item>
          <template v-if="draft.proxyPass.startsWith('https://')"
            ><el-divider content-position="left">SNI 设置</el-divider
            ><el-form-item label="启用 SNI"
              ><el-switch v-model="draft.sni" @change="sniTouched = true" /></el-form-item
            ><el-form-item v-if="draft.sni" label="SNI 域名"
              ><el-input
                v-model="draft.sslName"
                placeholder="留空使用代理地址中的主机名" /></el-form-item
            ><el-form-item label="验证上游证书"
              ><el-switch v-model="draft.sslVerify" /></el-form-item
          ></template> </el-tab-pane
        ><el-tab-pane label="高级设置" name="advanced">
          <el-form-item label="连接超时（秒）"
            ><el-input-number v-model="draft.connectTimeout" :min="1" :max="3600" /></el-form-item
          ><el-form-item label="读取超时（秒）"
            ><el-input-number v-model="draft.readTimeout" :min="1" :max="3600" /></el-form-item
          ><el-form-item label="发送超时（秒）"
            ><el-input-number v-model="draft.sendTimeout" :min="1" :max="3600"
          /></el-form-item>
          <el-divider content-position="left">缓存设置</el-divider
          ><el-form-item label="服务端缓存"><el-switch v-model="draft.cache" /></el-form-item
          ><el-form-item v-if="draft.cache" label="服务端缓存时间"
            ><div class="duration-input">
              <el-input-number v-model="draft.serverCacheTime" :min="1" :max="65535" /><el-select
                v-model="draft.serverCacheUnit"
                ><el-option v-for="u in units" :key="u.value" :value="u.value" :label="u.label"
              /></el-select></div
          ></el-form-item>
          <el-form-item label="浏览器缓存"
            ><el-radio-group
              :model-value="
                draft.cacheTime > 0 ? 'enable' : draft.cacheTime < 0 ? 'disable' : 'noModify'
              "
              @change="
                (value) =>
                  draft && (draft.cacheTime = value === 'enable' ? 4 : value === 'disable' ? -1 : 0)
              "
              ><el-radio-button value="enable">启用</el-radio-button
              ><el-radio-button value="disable">禁用</el-radio-button
              ><el-radio-button value="noModify">不修改</el-radio-button></el-radio-group
            ></el-form-item
          ><el-form-item v-if="draft.cacheTime > 0" label="浏览器缓存时间"
            ><div class="duration-input">
              <el-input-number v-model="draft.cacheTime" :min="1" :max="65535" /><el-select
                v-model="draft.cacheUnit"
                ><el-option v-for="u in units" :key="u.value" :value="u.value" :label="u.label"
              /></el-select></div
          ></el-form-item>
          <el-divider content-position="left">跨域设置</el-divider
          ><CORSFields v-model="draft.cors" />
          <el-divider content-position="left">内容替换</el-divider>
          <div v-for="(replacement, i) in draft.replaces" :key="i" class="replacement-row">
            <el-input
              v-model="replacement.from"
              :aria-label="`替换原文 ${i + 1}`"
              placeholder="替换原文"
            /><el-input
              v-model="replacement.to"
              :aria-label="`替换内容 ${i + 1}`"
              placeholder="替换内容"
            /><el-button type="danger" link @click="draft.replaces.splice(i, 1)">删除</el-button>
          </div>
          <el-button type="primary" plain @click="draft.replaces.push({ from: '', to: '' })"
            >添加替换</el-button
          >
        </el-tab-pane></el-tabs
      ></el-form
    ><template #footer
      ><el-button :disabled="busy" @click="open = false">取消</el-button
      ><el-button type="primary" :loading="busy" @click="submit">确认</el-button></template
    >
  </PanelDrawer>
  <PanelDrawer v-if="cache" title="代理缓存设置" @close="cache = undefined"
    ><el-form label-position="top"
      ><el-form-item label="共享内存（MiB）"
        ><el-input-number v-model="cache.memoryMb" :min="1" :max="1024" /></el-form-item
      ><el-form-item label="缓存容量（MiB）"
        ><el-input-number v-model="cache.maxSizeMb" :min="1" :max="1048576" /></el-form-item
      ><el-form-item label="不活跃过期时间（分钟）"
        ><el-input-number v-model="cache.inactiveMinutes" :min="1" :max="1051200"
      /></el-form-item>
      <p class="input-help">
        当前网站的缓存空间独立于其他网站。缓存内容不进入备份，缓存设置会备份。
      </p></el-form
    ><template #footer
      ><el-button @click="cache = undefined">取消</el-button
      ><el-button
        type="primary"
        :loading="busy"
        @click="
          async () => {
            if (await save('proxy-cache', cache)) cache = undefined;
          }
        "
        >保存</el-button
      ></template
    ></PanelDrawer
  >
  <PanelDrawer
    v-if="preview !== undefined"
    title="反向代理配置预览"
    size="large"
    @close="preview = undefined"
    ><p class="input-help">
      由表单生成的配置片段；关联的跨域 map、缓存区域和证书定义在完整配置中。
    </p>
    <CodemirrorPro
      :model-value="preview"
      mode="nginx"
      readonly
      label="反向代理配置"
      :height="550"
    /><template #footer
      ><el-button @click="preview = undefined">关闭</el-button></template
    ></PanelDrawer
  >
  <DeleteDialog ref="confirmRef" />
</template>
