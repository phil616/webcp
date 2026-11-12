import { storeToRefs } from 'pinia';
import { useInterfaceStore } from '@/stores/interface';
export const useGlobalStore = () => {
    const globalStore = useInterfaceStore();
    return { globalStore, ...storeToRefs(globalStore) };
};
