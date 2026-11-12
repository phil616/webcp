import { watch, onBeforeMount, onMounted, onBeforeUnmount } from 'vue';
import { useRoute } from 'vue-router';
import { useInterfaceStore } from '@/stores/interface';
const MOBILE_BREAKPOINT = 768;
export default () => {
    const route = useRoute(),
        store = useInterfaceStore();
    let previousWidth = 0;
    const syncLayout = (initial = false) => {
        const width = document.body.getBoundingClientRect().width;
        store.isMobile = width < MOBILE_BREAKPOINT;
        if (store.isMobile) {
            store.mobileOpen = false;
            store.withoutAnimation = true;
        } else if (width !== previousWidth) {
            if (width < 1200) store.isCollapsed = true;
            else if (!initial) store.isCollapsed = false;
            store.withoutAnimation = false;
        }
        previousWidth = width;
    };
    const resize = () => {
        if (!document.hidden) syncLayout();
    };
    watch(
        () => route.fullPath,
        () => {
            if (store.isMobile) {
                store.mobileOpen = false;
                store.withoutAnimation = false;
            }
        },
    );
    onBeforeMount(() => window.addEventListener('resize', resize));
    onMounted(() => syncLayout(true));
    onBeforeUnmount(() => window.removeEventListener('resize', resize));
};
