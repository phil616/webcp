<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { usePanelUI } from '@/composables/usePanelUI';
import useResize from './hooks/useResize';
const { isCollapsed, isMobile, isFullScreen, mobileOpen, withoutAnimation } = usePanelUI();
useResize();
const route = useRoute();
const contentRef = ref<HTMLElement>();
// This standalone layout scrolls its content region rather than the window.
watch(
    () => route.fullPath,
    () => contentRef.value?.scrollTo({ left: 0, top: 0 }),
    { flush: 'post' },
);
const classObj = computed(() => ({
    fullScreen: isFullScreen.value,
    hideSidebar: isMobile.value ? !mobileOpen.value : isCollapsed.value,
    openSidebar: isMobile.value ? mobileOpen.value : !isCollapsed.value,
    mobile: isMobile.value,
    mobileOpen: mobileOpen.value,
    withoutAnimation: withoutAnimation.value,
}));
</script>
<template>
    <div class="app-wrapper relative" :class="classObj">
        <button
            v-if="isMobile && mobileOpen"
            class="drawer-bg"
            aria-label="关闭导航"
            @click="mobileOpen = false"
        />
        <el-affix v-if="!isMobile && !isFullScreen" :offset="15" class="affix">
            <el-tooltip :content="isCollapsed ? '展开导航' : '收起导航'"
                ><el-button
                    class="collapse-button"
                    size="small"
                    circle
                    plain
                    :style="{ 'margin-left': isCollapsed ? '63px' : '168px', position: 'absolute' }"
                    :icon="isCollapsed ? 'ArrowRight' : 'ArrowLeft'"
                    :aria-label="isCollapsed ? '展开导航' : '收起导航'"
                    @click="
                        isCollapsed = !isCollapsed;
                        withoutAnimation = false;
                    "
            /></el-tooltip>
        </el-affix>
        <aside v-if="!isFullScreen" class="app-sidebar sidebar" aria-label="主导航">
            <slot name="sidebar" />
        </aside>
        <div class="main-container">
            <slot name="header" />
            <main ref="contentRef" class="app-main"><slot /></main>
            <slot v-if="!isFullScreen" name="footer" />
        </div>
    </div>
</template>
<style scoped lang="scss">
.app-wrapper {
    position: relative;
    width: 100%;
}

.drawer-bg {
    background-color: #000;
    opacity: 0.3;
    width: 100%;
    top: 0;
    height: 100%;
    position: absolute;
    z-index: 999;
}

.main-container {
    display: flex;
    flex-direction: column;
    position: relative;
    height: 100vh;
    height: 100dvh;
    min-width: 0;
    transition: margin-left 0.3s;
    margin-left: var(--panel-menu-width);
    background-color: var(--panel-main-bg-color-9);
    overflow-x: hidden;
}
.app-main {
    padding: 7px 20px;
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: auto;
}
.app-sidebar {
    z-index: 2;
    transition: width 0.3s;
    width: var(--panel-menu-width) !important;
    position: fixed;
    font-size: 0px;
    top: 0;
    bottom: 0;
    left: 0;
    overflow: hidden;
    .affix {
        z-index: 5;
    }
}

.hideSidebar {
    .main-container {
        margin-left: var(--panel-menu-hide-width);
    }
    .app-sidebar {
        width: var(--panel-menu-hide-width) !important;
    }
    .fixed-header {
        width: calc(100% - var(--panel-menu-hide-width));
    }
}

.fullScreen {
    .main-container {
        margin-left: 0px;
    }
}
// for mobile response 适配移动端
.mobile {
    .main-container {
        margin-left: 0px;
    }
    .app-sidebar {
        transition: transform 0.3s;
        width: var(--panel-menu-width) !important;
        background: #ffffff;
        z-index: 9999;
    }
    .app-footer {
        display: block;
        text-align: center;
    }
    &.openSidebar {
        position: fixed;
        top: 0;
    }
    &.hideSidebar {
        .app-sidebar {
            pointer-events: none;
            transition-duration: 0.3s;
            transform: translate3d(calc(0px - var(--panel-menu-width)), 0, 0);
        }
    }
}

.withoutAnimation {
    .main-container,
    .sidebar-container {
        transition: none;
    }
}

@media (max-width: 767px) {
    .app-main {
        padding-right: 12px;
        padding-left: 12px;
    }
}
</style>
