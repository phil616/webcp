<script setup lang="ts">
import { computed, reactive, ref, toRaw } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { Site } from '@/types';
import { defaultCORS, defaultHTTPS } from '@/components/website/shared';
import { useManager, request } from '@/api';
import WebsiteFields from '@/components/panel/WebsiteFields.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
const props = defineProps<{ site: Site | null }>();
const emit = defineEmits<{ close: [] }>();
const { state, busy, act } = useManager();
const editRevision = ref(state.value!.revision);
const form = ref<FormInstance>();
const draft = ref<Site>(
  props.site
    ? structuredClone(toRaw(props.site))
    : {
        id: '',
        headers: [],
        security: { followSymlinks: false, allowHiddenFiles: false },
        bindings: [],
        runDirectory: '/',
        index: ['index.html'],
        https: defaultHTTPS(),
        cors: defaultCORS(),
        realIp: { enabled: false, from: [], header: 'X-Real-IP' },
        proxies: [],
        redirects: [],
        proxyCache: { memoryMb: 5, maxSizeMb: 1024, inactiveMinutes: 60 },
        name: '',
        domains: [],
        kind: 'static',
        upstream: '',
        enabled: true,
        tlsId: '',
        luaBindings: {},
        createdAt: new Date().toISOString(),
      },
);
const domains = ref(draft.value.domains.join('\n'));
const model = computed(() => ({ ...draft.value, domainsText: domains.value }));
const rules = reactive<FormRules>({
  name: [{ required: true, message: '请输入网站名称', trigger: 'blur' }],
  domainsText: [{ required: true, message: '请输入域名或 IP', trigger: 'blur' }],
  kind: [{ required: true, message: '请选择网站类型', trigger: 'change' }],
  upstream: [
    {
      validator: (_rule, value, callback) =>
        draft.value.kind === 'proxy' && !value ? callback(new Error('请输入代理地址')) : callback(),
      trigger: 'blur',
    },
  ],
});
async function save() {
  if (!(await form.value?.validate().catch(() => false))) {
    return;
  }
  const value: Site = {
    ...draft.value,
    domains: domains.value.split(/[\s,]+/).filter(Boolean),
    bindings: domains.value
      .split(/[\s,]+/)
      .filter(Boolean)
      .map((domain) => ({ domain: domain.toLowerCase(), port: 80, ssl: false })),
    luaBindings: { ...draft.value.luaBindings },
  };
  for (const key of Object.keys(value.luaBindings))
    if (!value.luaBindings[key]) delete value.luaBindings[key];
  if (value.kind === 'proxy') {
    delete value.luaBindings.content;
    // Let the backend migrate the creation shortcut into the default proxy rule.
    (value as Partial<Site>).proxies = undefined;
  }
  const ok = await act(
    async () => {
      const saved = await request<Site>('/sites', 'POST', value, editRevision.value);
      draft.value.id = saved.id;
      draft.value.createdAt = saved.createdAt;
    },
    '保存成功',
    { deploy: true },
  );
  if (ok) {
    editRevision.value = state.value!.revision;
    emit('close');
  }
}
</script>
<template>
  <PanelDrawer :title="site ? '编辑网站' : '创建网站'" size="normal" @close="emit('close')"
    ><div class="form-width">
      <el-form ref="form" :model="model" :rules="rules" label-position="top" @submit.prevent="save"
        ><WebsiteFields v-model:draft="draft" v-model:domains="domains" section="basic" />
        <details class="form-section">
          <summary class="form-section-header">HTTPS（可选）</summary>
          <div class="form-section-body">
            <WebsiteFields v-model:draft="draft" v-model:domains="domains" section="tls" />
          </div></details
      ></el-form>
    </div>
    <template #footer
      ><el-button :disabled="busy" @click="emit('close')">取消</el-button
      ><el-button type="primary" :loading="busy" @click="save">确认</el-button></template
    ></PanelDrawer
  >
</template>
