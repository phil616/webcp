<template>
    <div>
        <el-card class="router_card p-1 sm:p-0">
            <div
                class="flex w-full flex-col justify-start sm:items-center items-start sm:justify-between sm:flex-row"
            >
                <el-radio-group :model-value="activeName" @change="handleChange">
                    <el-radio-button
                        class="router_card_button"
                        :class="{ 'opacity-50': button.disabled }"
                        :label="button.label"
                        :value="button.label"
                        :disabled="button.disabled"
                        v-for="(button, index) in buttonArray"
                        size="large"
                        :key="index"
                    >
                        <el-badge :value="button.count" v-if="button.count" is-dot>
                            <span>{{ button.label }}</span>
                        </el-badge>
                        <span v-else>{{ button.label }}</span>
                    </el-radio-button>
                </el-radio-group>
                <div class="flex flex-col gap-2 sm:flex-row">
                    <slot name="route-button"></slot>
                </div>
            </div>
        </el-card>
    </div>
</template>

<script lang="ts" setup>
import { openPage } from '@panel/utils/router';
import { computed } from 'vue';
import { useRouter } from 'vue-router';
defineOptions({ name: 'RouterButton' });
const props = defineProps<{
    buttons: Array<{
        label: string;
        path?: string;
        name?: string;
        disabled?: boolean;
        count?: number;
    }>;
}>();
const router = useRouter();
const buttonArray = computed(() => props.buttons);
// Derive selection from the router; nested paths must beat their parent.
const activeName = computed(() => {
    const route = router.currentRoute.value;
    const matches = buttonArray.value.filter((button) =>
        button.path
            ? route.path === button.path ||
              route.path.startsWith(button.path.replace(/\/$/, '') + '/')
            : button.name === route.name,
    );
    matches.sort((a, b) => (b.path?.length ?? 0) - (a.path?.length ?? 0));
    return matches[0]?.label ?? '';
});
const handleChange = async (label: string) => {
    const button = buttonArray.value.find((button) => button.label === label);
    if (!button || button.disabled) return;
    try {
        if (button.path) await openPage({ path: button.path });
        else if (button.name) await openPage({ name: button.name });
    } catch {
        /* The selected value continues to follow the current route. */
    }
};
</script>

<style lang="scss" scoped>
.router_card {
    --el-card-padding: 0;

    :deep(.el-card__body) {
        display: flex;
        justify-content: space-between;
        align-items: center;
    }
}

.router_card_button {
    :deep(.el-radio-button__inner) {
        min-width: 100px;
        height: 100%;
        background-color: var(--panel-button-active) !important;
        box-shadow: none !important;
        outline: none !important;
        border: 2px solid transparent !important;
        color: var(--el-text-color-regular) !important;
    }

    :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
        color: var(--panel-button-text-color) !important;
        background-color: var(--panel-button-bg-color) !important;
        border-color: var(--panel-color-primary) !important;
        border-radius: 4px;
    }
}
</style>
