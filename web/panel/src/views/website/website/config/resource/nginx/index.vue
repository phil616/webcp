<template>
    <div v-loading="loading">
        <el-alert class="mb-4" type="info" :closable="false" show-icon>
            <template #default>
                编辑当前网站草稿，保存前必须完整映射回 GUI。未知指令、受管配置冲突及不可表示的改动即使语法正确也会拒绝。
                端口使用网站端口，资源使用稳定路径；部署时自动转换。注释和排版在保存后规范化。
            </template>
        </el-alert>
        <el-alert v-if="notice?.error" class="mb-4" type="error" :closable="false" :title="notice.text" show-icon />
        <CodemirrorPro v-model="content" mode="nginx" :height="500" label="网站 Nginx 配置" :disabled="loading || busy" />
        <el-button type="primary" @click="submit()" class="mt-2.5" :disabled="!revision || loading || busy">
            {{ $t('nginx.saveAndReload') }}
        </el-button>
        <el-button @click="get()" :disabled="loading || busy">重新加载</el-button>
    </div>
</template>
<script lang="ts" setup>
import { onMounted, ref } from 'vue';
import { request, useManager, notify } from '@/api';
import CodemirrorPro from '@/vendor/1panel/components/codemirror-pro/index.vue';

const props = defineProps({
    id: {
        type: String,
        default: '',
    },
});

const { busy, act, notice } = useManager();
const loading = ref(false);
const content = ref('');
const revision = ref(0);

const get = async () => {
    loading.value = true;
    try {
        const data = await request<{ content: string; revision: number }>(`/sites/${props.id}/config`);
        content.value = data.content;
        revision.value = data.revision;
    } catch (error) {
        notify(error instanceof Error ? error.message : String(error), true);
    } finally {
        loading.value = false;
    }
};

const submit = async () => {
    const saved = await act(
        () => request(`/sites/${props.id}/config`, 'PUT', { content: content.value }, revision.value),
        '保存成功',
        { deploy: true },
    );
    if (saved) await get();
};

onMounted(get);
</script>
