<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import type { FormInstance } from 'element-plus';
import {
  CAModule,
  OfficialRouterButton,
  OfficialLayout,
  OfficialDrawer,
  OfficialOpDialog,
  OfficialKeyTypes,
} from '@panel/entry';
import { useManager, request, date } from '../api';
import type { Certificate } from '../types';
const { state, busy, act } = useManager();
const route = useRoute();
const section = computed(() =>
  route.path.endsWith('/clients')
    ? 'clients'
    : route.path.endsWith('/bindings')
      ? 'bindings'
      : 'ca',
);
const tabs = [
  { label: 'CA 管理', path: '/mtls' },
  { label: '客户端证书', path: '/mtls/clients' },
  { label: '网站绑定', path: '/mtls/bindings' },
];
const query = ref('');
const clients = computed(
  () =>
    state.value?.mtls.clients.filter((c) =>
      c.name.toLowerCase().includes(query.value.toLowerCase()),
    ) ?? [],
);
const authorities = computed(() => state.value?.mtls.authorities ?? []);
const bindings = computed(
  () =>
    state.value?.sites.map((site) => ({
      ...site,
      binding: state.value!.mtls.bindings.find((b) => b.siteId === site.id),
    })) ?? [],
);
const open = ref(false),
  bindingOpen = ref(false);
const form = ref<FormInstance>(),
  bindingForm = ref<FormInstance>();
const draft = reactive({ name: '', kind: 'client', caId: '', days: 365, keyType: 'EC256' });
const binding = reactive({ siteId: '', caId: '', enabled: true });
const confirmation = ref<{ acceptParams: (v: unknown) => void }>();
const required = [{ required: true, message: '此项为必填项', trigger: 'change' }];
function caName(id: string) {
  return authorities.value.find((c) => c.id === id)?.name ?? id;
}
function status(c: Certificate) {
  return c.revokedAt ? '已吊销' : Date.parse(c.notAfter) < Date.now() ? '已过期' : '有效';
}
function create() {
  Object.assign(draft, {
    name: '',
    kind: 'client',
    caId: authorities.value[0]?.id ?? '',
    days: 365,
    keyType: 'EC256',
  });
  open.value = true;
}
async function issue() {
  if (!(await form.value?.validate().catch(() => false))) return;
  if (await act(() => request('/mtls/issue', 'POST', draft), '客户端证书已签发'))
    open.value = false;
}
function revoke(c: Certificate) {
  confirmation.value?.acceptParams({
    title: '吊销客户端证书',
    msg: '吊销后此客户端将无法通过 mTLS 验证，吊销记录会保留在 CA 中。',
    names: [c.name],
    params: {},
    noMsg: true,
    api: async () => {
      if (
        !(await act(() => request(`/mtls/clients/${c.id}/revoke`, 'POST'), '客户端证书已吊销', {
          deploy: true,
        }))
      )
        throw new Error('吊销失败');
    },
  });
}
function editBinding(siteId: string) {
  const old = state.value?.mtls.bindings.find((b) => b.siteId === siteId);
  Object.assign(binding, { siteId, caId: old?.caId ?? '', enabled: old?.enabled ?? true });
  bindingOpen.value = true;
}
async function saveBinding() {
  if (!(await bindingForm.value?.validate().catch(() => false))) return;
  if (
    await act(() => request('/mtls/bindings', 'PUT', binding), '网站 mTLS 绑定已保存', {
      deploy: true,
    })
  )
    bindingOpen.value = false;
}
function unbind(siteId: string) {
  const site = state.value?.sites.find((s) => s.id === siteId);
  confirmation.value?.acceptParams({
    title: '解除 mTLS 绑定',
    msg: '解除后该网站不再校验客户端证书。',
    names: [site?.name ?? siteId],
    params: {},
    noMsg: true,
    api: async () => {
      if (
        !(await act(
          () => request('/mtls/bindings', 'PUT', { siteId, caId: '', enabled: false }),
          'mTLS 绑定已解除',
          { deploy: true },
        ))
      )
        throw new Error('解除失败');
    },
  });
}
</script>
<template>
  <div class="route-view">
    <OfficialRouterButton :buttons="tabs" />
    <CAModule v-if="section === 'ca'" :embedded="true" />
    <OfficialLayout v-else-if="section === 'clients'" title="客户端证书">
      <template #leftToolBar
        ><el-button type="primary" :disabled="!authorities.length" @click="create"
          >签发证书</el-button
        ></template
      >
      <template #rightToolBar
        ><el-input v-model="query" clearable placeholder="搜索客户端证书"
      /></template>
      <template #main>
        <el-alert
          v-if="!authorities.length"
          title="请先在 CA 管理中创建或导入 mTLS CA。"
          type="info"
          :closable="false"
          class="mb-4"
        />
        <el-table :data="clients" v-loading="busy">
          <el-table-column prop="name" label="名称" min-width="160" show-overflow-tooltip />
          <el-table-column label="签发 CA" min-width="150"
            ><template #default="{ row }">{{ caName(row.caId) }}</template></el-table-column
          >
          <el-table-column prop="serial" label="序列号" min-width="180" show-overflow-tooltip />
          <el-table-column label="状态" width="100"
            ><template #default="{ row }"
              ><el-tag :type="status(row as Certificate) === '有效' ? 'success' : 'danger'">{{
                status(row as Certificate)
              }}</el-tag></template
            ></el-table-column
          >
          <el-table-column label="过期时间" min-width="180"
            ><template #default="{ row }">{{ date(row.notAfter) }}</template></el-table-column
          >
          <el-table-column label="操作" width="200" fixed="right"
            ><template #default="{ row }">
              <el-button
                link
                type="primary"
                tag="a"
                :href="`/api/mtls/certificates/${row.id}/download?key=true`"
                >下载</el-button
              >
              <el-button
                link
                type="primary"
                :disabled="!!row.revokedAt"
                @click="revoke(row as Certificate)"
                >吊销</el-button
              >
            </template></el-table-column
          >
        </el-table>
      </template>
    </OfficialLayout>
    <OfficialLayout v-else title="网站绑定">
      <template #leftToolBar><el-text>mTLS 客户端验证</el-text></template>
      <template #main>
        <el-table :data="bindings" v-loading="busy">
          <el-table-column prop="name" label="网站" min-width="140" />
          <el-table-column label="域名" min-width="200"
            ><template #default="{ row }">{{ row.domains.join(', ') }}</template></el-table-column
          >
          <el-table-column label="HTTPS" width="110"
            ><template #default="{ row }"
              ><el-tag :type="row.tlsId ? 'success' : 'info'">{{
                row.tlsId ? '已启用' : '未启用'
              }}</el-tag></template
            ></el-table-column
          >
          <el-table-column label="信任 CA" min-width="150"
            ><template #default="{ row }">{{
              row.binding ? caName(row.binding.caId) : '—'
            }}</template></el-table-column
          >
          <el-table-column label="mTLS 状态" width="110"
            ><template #default="{ row }"
              ><el-tag :type="row.binding?.enabled ? 'success' : 'info'">{{
                row.binding?.enabled ? '已启用' : '未启用'
              }}</el-tag></template
            ></el-table-column
          >
          <el-table-column label="操作" width="200" fixed="right"
            ><template #default="{ row }"
              ><el-button link type="primary" @click="editBinding(row.id)">设置</el-button
              ><el-button v-if="row.binding" link type="primary" @click="unbind(row.id)"
                >解除绑定</el-button
              ></template
            ></el-table-column
          >
        </el-table>
      </template>
    </OfficialLayout>
    <OfficialDrawer v-model="open" header="签发客户端证书" size="large">
      <el-form ref="form" label-position="top" :model="draft" :disabled="busy">
        <el-form-item label="名称（Common Name）" prop="name" :rules="required"
          ><el-input v-model.trim="draft.name" maxlength="128"
        /></el-form-item>
        <el-form-item label="签发 CA" prop="caId" :rules="required"
          ><el-select v-model="draft.caId"
            ><el-option
              v-for="ca in authorities"
              :key="ca.id"
              :value="ca.id"
              :label="ca.name" /></el-select
        ></el-form-item>
        <el-form-item label="密钥算法" prop="keyType" :rules="required"
          ><el-select v-model="draft.keyType"
            ><el-option
              v-for="key in OfficialKeyTypes"
              :key="key.value"
              :value="key.value"
              :label="key.label" /></el-select
        ></el-form-item>
        <el-form-item label="有效期（天）" prop="days" :rules="required"
          ><el-input-number v-model="draft.days" :min="1" :max="3650"
        /></el-form-item>
      </el-form>
      <template #footer
        ><el-button :disabled="busy" @click="open = false">取消</el-button
        ><el-button type="primary" :loading="busy" @click="issue">确认</el-button></template
      >
    </OfficialDrawer>
    <OfficialDrawer v-model="bindingOpen" header="mTLS 网站绑定" size="large">
      <el-form ref="bindingForm" label-position="top" :model="binding" :disabled="busy">
        <el-alert
          type="info"
          :closable="false"
          title="请先在网站 HTTPS 设置中配置服务器证书，并选择跳转 HTTPS 或禁止 HTTP。"
          class="mb-4"
        />
        <el-form-item label="网站"
          ><el-text>{{
            state?.sites.find((s) => s.id === binding.siteId)?.name
          }}</el-text></el-form-item
        >
        <el-form-item label="信任 CA" prop="caId" :rules="required"
          ><el-select v-model="binding.caId"
            ><el-option
              v-for="ca in authorities"
              :key="ca.id"
              :value="ca.id"
              :label="ca.name" /></el-select
        ></el-form-item>
        <el-form-item label="启用 mTLS"><el-switch v-model="binding.enabled" /></el-form-item>
        <el-form-item v-if="binding.caId" label="吊销列表"
          ><el-button
            link
            type="primary"
            tag="a"
            :href="`/api/mtls/certificates/${binding.caId}/download?crl=true`"
            >下载 CRL</el-button
          ></el-form-item
        >
      </el-form>
      <template #footer
        ><el-button :disabled="busy" @click="bindingOpen = false">取消</el-button
        ><el-button type="primary" :loading="busy" @click="saveBinding">确认</el-button></template
      >
    </OfficialDrawer>
    <OfficialOpDialog ref="confirmation" />
  </div>
</template>
