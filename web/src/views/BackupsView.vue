<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import ConfirmDialog from '@/vendor/1panel/components/confirm-dialog/index.vue';
import type { FormInstance, FormRules } from 'element-plus';
import type { Backup, S3 } from '@/types';
import { useManager, request, date, size } from '@/api';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import CardWithHeader from '@/vendor/1panel/components/card-with-header/index.vue';
import TableRefresh from '@/vendor/1panel/components/table/TableRefresh.vue';
import TableSearch from '@/vendor/1panel/components/table/TableSearch.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import PanelDrawer from '@/components/panel/PanelDrawer.vue';
const { state, busy, act } = useManager();
const records = ref<Backup[]>([]),
  remote = ref(false),
  search = ref(''),
  loading = ref(false),
  loadError = ref('');
const editing = ref(false),
  creating = ref(false),
  toS3 = ref(false),
  revision = ref(0);
const config = ref<S3>({ ...state.value!.s3, secretKey: '' });
const form = ref<FormInstance>(),
  confirmRef = ref<InstanceType<typeof ConfirmDialog>>();
const pendingRestore = ref<{ record: Backup; remote: boolean; revision: number }>();
const configured = computed(() => !!state.value?.s3.endpoint);
const items = computed(() => records.value.filter((item) => item.id.includes(search.value ?? '')));
const rules: FormRules = {
  endpoint: [
    { required: true, message: '请输入 Endpoint', trigger: 'blur' },
    { type: 'url', message: '请输入包含 http(s) 协议的地址', trigger: 'blur' },
  ],
  bucket: [{ required: true, message: '请输入存储桶名称', trigger: 'blur' }],
  accessKey: [{ required: true, message: '请输入 Access Key', trigger: 'blur' }],
  secretKey: [
    {
      validator: (_rule, value, callback) =>
        !configured.value && !value ? callback(new Error('请输入 Secret Key')) : callback(),
      trigger: 'blur',
    },
  ],
};
async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    records.value = await request<Backup[]>(`/backups?remote=${remote.value}`);
  } catch (error) {
    records.value = [];
    loadError.value = String(error);
  } finally {
    loading.value = false;
  }
}
onMounted(load);
function openSettings() {
  revision.value = state.value!.revision;
  config.value = { ...state.value!.s3, secretKey: '' };
  editing.value = true;
}
async function save() {
  if (!(await form.value?.validate().catch(() => false))) return;
  if (await act(() => request('/s3', 'PUT', config.value, revision.value), '保存成功'))
    editing.value = false;
}
async function create() {
  if (await act(() => request('/backups', 'POST', { remote: toS3.value }), '备份成功')) {
    creating.value = false;
    await load();
  }
}
function restore(record: Backup) {
  pendingRestore.value = { record, remote: remote.value, revision: state.value!.revision };
  confirmRef.value?.acceptParams({
    header: '恢复备份',
    operationInfo: `备份：${record.id}\n时间：${date(record.createdAt)}\n恢复会停止 OpenResty，替换全部网站文件、配置、证书与 S3 凭据。完成后需检查并应用配置。`,
    submitInputInfo: '恢复',
  });
}
async function performRestore() {
  const pending = pendingRestore.value;
  if (!pending) return;
  if (
    await act(
      () =>
        request(
          `/backups/${pending.record.id}/restore`,
          'POST',
          { remote: pending.remote },
          pending.revision,
        ),
      '恢复成功，配置已恢复为草稿。请检查后启动 OpenResty。',
    )
  ) {
    remote.value = false;
    await load();
  }
  pendingRestore.value = undefined;
}
function download(record: Backup) {
  const link = document.createElement('a');
  link.href = `/api/backups/${record.id}/download`;
  link.download = '';
  link.click();
}
const operations: FuTableOperationButton<Backup>[] = [
  {
    label: '恢复',
    click: (record) => {
      void restore(record);
    },
  },
  { label: '下载', show: () => !remote.value, click: download },
];
</script>
<template>
  <div class="route-view">
    <LayoutContent
      ><template #app
        ><CardWithHeader header="S3 兼容存储"
          ><template #header-r
            ><el-button
              type="primary"
              link
              :disabled="busy || !configured"
              @click="
                act(
                  () => request('/s3/check', 'POST'),
                  '连接测试成功：写入、回读、列举和删除权限均可用。',
                )
              "
              >测试连接</el-button
            ><el-button type="primary" link @click="openSettings">设置</el-button></template
          ><template #body
            ><el-descriptions :column="2"
              ><el-descriptions-item label="状态" card-type="status"
                ><el-tag size="small" :type="configured ? 'success' : 'info'">{{
                  configured ? '已配置' : '未配置'
                }}</el-tag></el-descriptions-item
              ><el-descriptions-item label="Endpoint">{{
                state?.s3.endpoint || '—'
              }}</el-descriptions-item
              ><el-descriptions-item label="存储桶">{{
                state?.s3.bucket || '—'
              }}</el-descriptions-item
              ><el-descriptions-item label="对象前缀">{{
                state?.s3.prefix || '—'
              }}</el-descriptions-item></el-descriptions
            ></template
          ></CardWithHeader
        ></template
      ><template #leftToolBar
        ><el-button type="primary" @click="creating = true">创建备份</el-button
        ><el-button type="primary" plain @click="openSettings">存储设置</el-button></template
      ><template #rightToolBar
        ><el-select v-model="remote" class="filter-select" aria-label="备份来源" @change="load"
          ><el-option :value="false" label="本地备份" /><el-option
            :value="true"
            label="S3 备份"
            :disabled="!configured" /></el-select
        ><TableSearch v-model:search-name="search" placeholder="搜索备份 ID" /><TableRefresh
          aria-label="刷新备份"
          :disabled="loading"
          @search="load" /></template
      ><template #main
        ><el-alert
          v-if="loadError"
          type="error"
          :title="loadError"
          :closable="false"
          class="mb-4"
        /><ComplexTable :data="items" :loading="loading"
          ><el-table-column label="备份名称" card-type="name" min-width="250"
            ><template #default="{ row }"
              ><div class="table-name">
                <strong class="mono">{{ row.id }}</strong
                ><el-text class="table-subtitle" truncated :title="row.sha256"
                  >SHA-256: {{ row.sha256 }}</el-text
                >
              </div></template
            ></el-table-column
          ><el-table-column label="备份时间" min-width="180" card-type="content"
            ><template #default="{ row }">{{ date(row.createdAt) }}</template></el-table-column
          ><el-table-column label="大小" width="110" card-type="content"
            ><template #default="{ row }">{{ size(row.size) }}</template></el-table-column
          ><el-table-column label="存储位置" width="130" card-type="content"
            ><template #default="{ row }">{{
              remote ? 'S3' : row.remote ? '本地 + S3' : '本地'
            }}</template></el-table-column
          ><el-table-column label="操作" card-type="button" width="140" fixed="right"
            ><template #default="{ row }"
              ><TableOperationActions
                :buttons="operations"
                :row="row as Backup" /></template></el-table-column
          ><template #footerLeft>备份包含配置、网站文件和私钥，请妥善保存。</template></ComplexTable
        ></template
      ></LayoutContent
    >
    <PanelDrawer v-if="editing" title="S3 存储设置" @close="editing = false"
      ><el-form
        ref="form"
        :model="config"
        :rules="rules"
        label-position="top"
        @submit.prevent="save"
        ><el-form-item label="Endpoint" prop="endpoint"
          ><div class="field-control">
            <el-input v-model="config.endpoint" placeholder="https://s3.example.com" /><span
              class="input-help"
              >包含 http(s) 协议，不含路径，存储桶需提前创建。</span
            >
          </div></el-form-item
        ><el-form-item label="存储桶" prop="bucket"
          ><el-input v-model="config.bucket" placeholder="website-backups" /></el-form-item
        ><el-form-item label="Region"
          ><el-input v-model="config.region" placeholder="us-east-1" /></el-form-item
        ><el-form-item label="对象前缀"
          ><el-input v-model="config.prefix" placeholder="resty/production" /></el-form-item
        ><el-form-item label="Access Key" prop="accessKey"
          ><el-input v-model="config.accessKey" autocomplete="off" /></el-form-item
        ><el-form-item label="Secret Key" prop="secretKey"
          ><el-input
            v-model="config.secretKey"
            type="password"
            show-password
            autocomplete="new-password"
            :placeholder="configured ? '留空保留已有密钥' : '请输入 Secret Key'" /></el-form-item
        ><el-form-item label="寻址方式"
          ><el-switch v-model="config.pathStyle" active-text="Path-style" /><span class="input-help"
            >适用于 MinIO 等 S3 兼容服务。</span
          ></el-form-item
        ></el-form
      ><template #footer
        ><el-button :disabled="busy" @click="editing = false">取消</el-button
        ><el-button type="primary" :loading="busy" @click="save">保存</el-button></template
      ></PanelDrawer
    >
    <PanelDrawer v-if="creating" title="创建备份" @close="creating = false"
      ><el-form label-position="top"
        ><el-form-item label="备份内容"
          ><el-checkbox :model-value="true" disabled
            >管理配置与网站文件（完整备份）</el-checkbox
          ></el-form-item
        ><el-form-item label="存储位置"
          ><el-checkbox v-model="toS3" :disabled="!configured">同时上传至 S3</el-checkbox
          ><span v-if="!configured" class="input-help"
            >请先配置 S3 存储；本地备份始终保留。</span
          ></el-form-item
        ><el-descriptions :column="1" border
          ><el-descriptions-item label="网站">{{ state?.sites.length }} 个</el-descriptions-item
          ><el-descriptions-item label="Lua 资源">{{ state?.lua.length }} 个</el-descriptions-item
          ><el-descriptions-item label="证书"
            >{{ state?.certificates.length }} 份</el-descriptions-item
          ></el-descriptions
        ><el-alert
          class="mt-4"
          type="info"
          show-icon
          :closable="false"
          title="备份包含证书私钥和 S3 凭据；备份期间请暂停外部部署写入，以获得一致的文件快照。" /></el-form
      ><template #footer
        ><el-button :disabled="busy" @click="creating = false">取消</el-button
        ><el-button type="primary" :loading="busy" @click="create">确认</el-button></template
      ></PanelDrawer
    ><ConfirmDialog
      ref="confirmRef"
      @confirm="performRestore"
      @cancel="pendingRestore = undefined"
    />
  </div>
</template>
