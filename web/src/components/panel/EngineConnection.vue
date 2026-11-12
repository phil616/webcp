<script setup lang="ts">
import { onMounted, reactive, ref, watch, nextTick } from 'vue';
import { request, useManager, notify } from '@/api';
import CardWithHeader from '@/vendor/1panel/components/card-with-header/index.vue';
type Connection = {
  containerId?: string;
  binary?: string;
  luaJit?: string;
  mainConfig?: string;
  version: number;
  mode: 'managed' | 'existing';
  container: string;
  image: string;
  httpBind: string;
  httpsBind: string;
};
type Choice = { id: string; name: string; image: string; status: string };
type Probe = { compatible: boolean; problems: string[]; connection?: Connection };
const { refresh } = useManager();
const connection = ref<Connection>();
const initializing = ref(true);
const draft = reactive<Connection>({
  version: 1,
  mode: 'managed',
  container: 'webcp-openresty',
  image: '1panel/openresty:1.27.1.2-5-1-focal',
  httpBind: '0.0.0.0:80',
  httpsBind: '0.0.0.0:443',
});
const choices = ref<Choice[]>([]),
  busy = ref(false),
  error = ref(''),
  probe = ref<Probe>();
watch(
  () => draft.container,
  () => {
    draft.containerId = undefined;
  },
);
watch(
  () => JSON.stringify(draft),
  () => {
    probe.value = undefined;
  },
);
async function load() {
  try {
    connection.value = await request<Connection>('/runtime/connection');
    Object.assign(draft, connection.value);
  } catch (reason) {
    error.value = String(reason);
  } finally {
    initializing.value = false;
  }
}
async function list() {
  try {
    choices.value = await request<Choice[]>('/runtime/containers');
    error.value = '';
  } catch (reason) {
    error.value = String(reason);
  }
}
async function check() {
  busy.value = true;
  error.value = '';
  try {
    const result = await request<Probe>('/runtime/connection/probe', 'POST', draft);
    if (result.connection) Object.assign(draft, result.connection);
    await nextTick();
    probe.value = result;
  } catch (reason) {
    error.value = String(reason);
  } finally {
    busy.value = false;
  }
}
async function save() {
  busy.value = true;
  error.value = '';
  try {
    await request('/runtime/connection', 'PUT', draft, draft.version);
    await load();
    await refresh();
    notify('容器连接已保存');
  } catch (reason) {
    error.value = String(reason);
  } finally {
    busy.value = false;
  }
}
function choose(name: string) {
  const container = choices.value.find((item) => item.name === name);
  if (container) draft.image = container.image;
}
onMounted(() => {
  void load();
  void list();
});
</script>
<template>
  <CardWithHeader header="容器连接"
    ><template #body
      ><el-alert
        v-if="error"
        :title="error"
        type="error"
        :closable="false"
        show-icon
        class="mb-4"
      /><el-form label-position="top" class="form-width" :disabled="busy || initializing"
        ><el-form-item label="连接方式"
          ><el-radio-group v-model="draft.mode"
            ><el-radio-button value="managed">创建或使用托管容器</el-radio-button
            ><el-radio-button value="existing">连接已有容器</el-radio-button></el-radio-group
          ></el-form-item
        ><el-form-item v-if="draft.mode === 'managed'" label="容器名称"
          ><el-input v-model="draft.container" /></el-form-item
        ><el-form-item v-else label="已有容器"
          ><div class="field-control">
            <el-select
              v-model="draft.container"
              filterable
              allow-create
              default-first-option
              @change="choose"
              ><el-option
                v-for="item in choices"
                :key="item.id"
                :label="`${item.name} · ${item.status}`"
                :value="item.name" /></el-select
            ><el-button class="mt-2" @click="list">刷新容器列表</el-button>
          </div></el-form-item
        ><el-form-item label="OpenResty 镜像"
          ><el-input
            v-model="draft.image"
            :readonly="draft.mode === 'existing'"
            placeholder="可填写自己的 OpenResty 镜像"
          /><span class="input-help" v-if="draft.mode === 'managed'"
            >Docker 中没有 OpenResty 时，应用配置会使用此镜像创建容器。</span
          ></el-form-item
        ><el-row :gutter="16"
          ><el-col :xs="24" :sm="12"
            ><el-form-item label="HTTP 发布地址"
              ><el-input v-model="draft.httpBind" placeholder="0.0.0.0:80" /></el-form-item></el-col
          ><el-col :xs="24" :sm="12"
            ><el-form-item label="HTTPS 发布地址"
              ><el-input
                v-model="draft.httpsBind"
                placeholder="0.0.0.0:443" /></el-form-item></el-col></el-row
        ><el-alert
          v-if="draft.mode === 'existing'"
          type="info"
          :closable="false"
          show-icon
          title="WebCP 将接管主配置：首次应用前备份原配置，不重建所选容器。原网站配置将由 WebCP 生成的配置替换。"
          class="mb-4"
        /><el-collapse v-if="draft.mode === 'existing'" class="mb-4"
          ><el-collapse-item title="高级连接设置" name="paths"
            ><el-form-item label="OpenResty 程序路径"
              ><el-input v-model="draft.binary" placeholder="自动检测" /></el-form-item
            ><el-form-item label="LuaJIT 程序路径"
              ><el-input v-model="draft.luaJit" placeholder="自动检测" /></el-form-item
            ><el-form-item label="容器内主配置路径"
              ><el-input
                v-model="draft.mainConfig"
                placeholder="从运行进程自动检测" /></el-form-item></el-collapse-item></el-collapse
        ><el-alert
          v-if="probe"
          :type="probe.compatible ? 'success' : 'warning'"
          :title="probe.compatible ? '容器检测通过' : '容器需要完成以下准备'"
          :closable="false"
          show-icon
          ><ul v-if="probe.problems.length">
            <li v-for="problem in probe.problems" :key="problem">{{ problem }}</li>
          </ul></el-alert
        >
        <div class="mt-4">
          <el-button @click="check" :loading="busy">检测连接</el-button
          ><el-button type="primary" @click="save" :loading="busy" :disabled="!connection"
            >保存连接</el-button
          >
        </div>
        <p class="input-help mt-3">保存只切换管理目标。配置部署仍通过“校验并应用”执行。</p></el-form
      ></template
    ></CardWithHeader
  >
</template>
