import { router } from '@/router';
import type { LocationQueryRaw, RouteLocationRaw } from 'vue-router';
export { router as panelRouter };
export const routerToName = (name: string) =>
    router.push({
        name: name === 'SSL' && router.currentRoute.value.path.startsWith('/mtls') ? 'MTLS' : name,
    });
export const routerToPath = (path: string) => router.push(path);
export const routerToNameWithQuery = (name: string, query: LocationQueryRaw = {}) =>
    router.push({ name, query });
export const openPage = (to: RouteLocationRaw) => router.push(to);
export const routerToFileWithPath = (path: string) =>
    router.push({ path: '/sites', query: { directory: path } });
