import { storeToRefs } from 'pinia';
import { useInterfaceStore } from '@/stores/interface';
export function usePanelUI() {
  return storeToRefs(useInterfaceStore());
}
