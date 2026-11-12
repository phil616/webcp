<script setup lang="ts">
import { computed, ref } from 'vue';
import { errorPages, defaultErrorPage } from '@/error-pages';
import { navigate } from '@/navigation';
import type { Site } from '@/types';
import { useManager } from '@/api';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';
const draft = defineModel<Site>('draft', { required: true });
const domains = defineModel<string>('domains', { required: true });
const props = defineProps<{ section: string; globalPages?: Record<string, string> }>();
const pages = defineModel<Record<string, string>>('pages', { default: () => ({}) });
const { state } = useManager();
const code = ref('404');
const servers = computed(
  () => state.value?.certificates.filter((c) => c.kind === 'server' && !c.revokedAt) ?? [],
);
const blocks = computed(() => state.value?.lua.filter((l) => l.kind === 'block') ?? []);
const phases = [
  { id: 'access', label: '访问阶段（access）' },
  { id: 'content', label: '内容生成（content）' },
  { id: 'header_filter', label: '响应头（header_filter）' },
  { id: 'body_filter', label: '响应体（body_filter）' },
  { id: 'log', label: '日志阶段（log）' },
];
function override(enabled: boolean) {
  if (enabled)
    pages.value[code.value] = props.globalPages?.[code.value] ?? defaultErrorPage(code.value);
  else delete pages.value[code.value];
}
</script>
<template>
  <template v-if="section === 'basic'"
    ><el-form-item label="网站类型" prop="kind"
      ><el-radio-group v-model="draft.kind"
        ><el-radio-button value="static">静态网站</el-radio-button
        ><el-radio-button value="proxy">反向代理</el-radio-button></el-radio-group
      ></el-form-item
    ><el-form-item label="网站名称" prop="name"
      ><el-input v-model="draft.name" maxlength="128" placeholder="请输入网站名称" /></el-form-item
    ><el-form-item label="域名 / IP" prop="domainsText"
      ><div class="field-control">
        <el-input
          v-model="domains"
          type="textarea"
          :rows="4"
          placeholder="example.com&#10;www.example.com"
        /><span class="input-help"
          >每行一个域名或 IP，不含协议和端口；支持通配域名，国际域名请使用 punycode。</span
        >
      </div></el-form-item
    ><el-form-item v-if="draft.kind === 'proxy'" label="代理地址" prop="upstream"
      ><div class="field-control">
        <el-input v-model="draft.upstream" placeholder="http://192.168.1.10:3000" /><span
          class="input-help"
          >仅支持一个 HTTP(S) 源站。HTTPS 源站会验证证书，请勿使用自签名上游证书。</span
        >
      </div></el-form-item
    ><el-form-item label="网站状态"
      ><el-switch v-model="draft.enabled" active-text="启用" inactive-text="停用" /></el-form-item
  ></template>
  <template v-else-if="section === 'tls'"
    ><el-alert
      class="mb-4"
      type="info"
      show-icon
      :closable="false"
      title="选择服务器证书后自动启用 HTTPS，并将 HTTP 重定向到 HTTPS。"
    /><el-form-item label="服务器证书"
      ><el-select v-model="draft.tlsId" clearable placeholder="不启用 HTTPS"
        ><el-option
          v-for="cert in servers"
          :key="cert.id"
          :value="cert.id"
          :label="`${cert.name}（${cert.domains.join(', ')}）`" /></el-select></el-form-item
    ><el-button type="primary" link @click="navigate('certificates')"
      >前往证书管理</el-button
    ></template
  >
  <template v-else-if="section === 'lua'"
    ><el-alert
      type="info"
      :closable="false"
      show-icon
      title='每个阶段可绑定一个代码块。代码块通过 require("模块名") 复用资源库中的模块。'
      class="mb-4"
    /><el-form-item v-for="phase in phases" :key="phase.id" :label="phase.label"
      ><el-select
        v-model="draft.luaBindings[phase.id]"
        clearable
        placeholder="不绑定"
        :disabled="phase.id === 'content' && draft.kind === 'proxy'"
        ><el-option
          v-for="block in blocks"
          :key="block.id"
          :value="block.id"
          :label="block.name" /></el-select
    ></el-form-item>
    <p class="input-help">内容生成阶段只适用于静态网站；反向代理由源站提供内容。</p></template
  >
  <template v-else-if="section === 'pages'"
    ><el-alert
      title="默认继承全局错误页面，启用覆盖后仅影响当前网站。"
      type="info"
      :closable="false"
      show-icon
      class="mb-4"
    /><el-form-item label="错误页面"
      ><el-select v-model="code"
        ><el-option
          v-for="status in errorPages"
          :key="status.id"
          :value="status.id"
          :label="status.label" /></el-select></el-form-item
    ><el-form-item label="覆盖全局页面"
      ><el-switch
        :model-value="pages[code] !== undefined"
        @change="(value) => override(Boolean(value))" /></el-form-item
    ><CodemirrorPro
      v-if="pages[code] !== undefined"
      v-model="pages[code]!"
      :key="code"
      mode="html"
      label="网站错误页面 HTML"
      :height="350"
    />
    <p v-else class="muted-text">当前使用全局 {{ code }} 页面。</p></template
  >
  <template v-else-if="section === 'directory'"
    ><el-descriptions :column="1" border
      ><el-descriptions-item label="网站 ID"
        ><el-text class="mono" truncated>{{ draft.id }}</el-text></el-descriptions-item
      ><el-descriptions-item label="网站目录"
        ><el-text class="mono">sites/{{ draft.id }}</el-text></el-descriptions-item
      ><el-descriptions-item label="默认文档">index.html</el-descriptions-item></el-descriptions
    ><el-alert
      class="mt-4"
      type="info"
      :closable="false"
      show-icon
      title="在网站列表中点击「目录」可管理文件。网站文件保存后即时生效；静态站点仅查找实际文件，不做伪静态重写。"
  /></template>
</template>
