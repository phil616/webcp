<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Site, ResponseHeader } from '@/types';
import { clone, useWebsiteSection } from './shared';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import { errorPages } from '@/error-pages';
const props = defineProps<{ site: Site }>();
const { busy, save, resetRevision } = useWebsiteSection(() => props.site);
const headers = ref<ResponseHeader[]>(clone(props.site.headers));
const security = ref(clone(props.site.security));
const scopes = computed(() => [
  { value: 'server', label: 'server（网站）' },
  { value: 'root', label: '默认 location /' },
  ...props.site.proxies.map((p) => ({ value: `proxy:${p.id}`, label: `代理 · ${p.name}` })),
  ...errorPages.map((p) => ({ value: `error:${p.id}`, label: `错误页面 · ${p.id}` })),
]);
function reset() {
  headers.value = clone(props.site.headers);
  security.value = clone(props.site.security);
  resetRevision();
}
function add() {
  headers.value = [
    ...headers.value,
    {
      scope: 'server',
      tls: !!props.site.tlsId,
      name: '',
      value: '',
      always: true,
    },
  ];
}
function remove(row: ResponseHeader) {
  headers.value = headers.value.filter((header) => header !== row);
}
</script>
<template>
  <el-form label-position="top" class="form-width" :disabled="busy" @submit.prevent>
    <el-form-item label="允许跟随符号链接"
      ><el-switch v-model="security.followSymlinks" aria-label="允许跟随符号链接"
    /></el-form-item>
    <el-form-item label="允许访问隐藏文件"
      ><el-switch v-model="security.allowHiddenFiles" aria-label="允许访问隐藏文件"
    /></el-form-item>
  </el-form>
  <p class="section-description">
    响应头遵循 OpenResty 的 add_header 继承规则。需要在代理、默认 location
    或错误页面中单独生效时，请选择对应范围。未启用的 HTTPS 或规则会保留设置。
  </p>
  <div class="section-toolbar">
    <el-button type="primary" plain :disabled="busy" @click="add">添加响应头</el-button>
  </div>
  <ComplexTable :data="headers" :row-key="(row) => String(headers.indexOf(row))">
    <el-table-column label="协议" width="130" card-type="content"
      ><template #default="{ row }">
        <el-select v-model="row.tls" aria-label="响应头协议" :disabled="busy"
          ><el-option :value="false" label="HTTP" /><el-option :value="true" label="HTTPS"
        /></el-select> </template
    ></el-table-column>
    <el-table-column label="作用范围" min-width="200" card-type="content"
      ><template #default="{ row }">
        <el-select v-model="row.scope" aria-label="响应头作用范围" :disabled="busy"
          ><el-option
            v-for="scope in scopes"
            :key="scope.value"
            :value="scope.value"
            :label="scope.label"
        /></el-select> </template
    ></el-table-column>
    <el-table-column label="名称" min-width="180" card-type="name"
      ><template #default="{ row }"
        ><el-input
          v-model="row.name"
          aria-label="响应头名称"
          maxlength="128"
          :disabled="busy" /></template
    ></el-table-column>
    <el-table-column label="值" min-width="220" card-type="content"
      ><template #default="{ row }"
        ><el-input
          v-model="row.value"
          aria-label="响应头值"
          maxlength="8192"
          :disabled="busy" /></template
    ></el-table-column>
    <el-table-column label="Always" width="100" card-type="content"
      ><template #default="{ row }"
        ><el-switch v-model="row.always" aria-label="始终发送响应头" :disabled="busy" /></template
    ></el-table-column>
    <el-table-column label="操作" width="90" card-type="button"
      ><template #default="{ row }"
        ><el-button type="danger" link :disabled="busy" @click="remove(row as ResponseHeader)"
          >删除</el-button
        ></template
      ></el-table-column
    >
  </ComplexTable>
  <div class="mt-4">
    <el-button type="primary" :loading="busy" @click="save('advanced', { headers, security })"
      >保存</el-button
    ><el-button :disabled="busy" @click="reset">重置</el-button>
  </div>
</template>
