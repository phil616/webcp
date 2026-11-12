import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router';
const ssl = () => import('./views/CertificatesView.vue');
const mtls = () => import('./views/MTLSView.vue');
const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/sites' },
  {
    path: '/sites',
    name: 'Website',
    component: () => import('./views/SitesView.vue'),
    meta: { section: 'sites' },
  },
  { path: '/websites/ssl', name: 'SSL', component: ssl, meta: { section: 'certificates' } },
  { path: '/websites/ssl/ca', name: 'SSL-CA', component: ssl, meta: { section: 'certificates' } },
  {
    path: '/websites/ssl/acme-account',
    name: 'SSL-ACME',
    component: ssl,
    meta: { section: 'certificates' },
  },
  {
    path: '/websites/ssl/dns-account',
    name: 'SSL-DNS',
    component: ssl,
    meta: { section: 'certificates' },
  },
  { path: '/certificates', redirect: '/websites/ssl' },
  { path: '/mtls', name: 'MTLS', component: mtls, meta: { section: 'mtls' } },
  { path: '/mtls/ca', name: 'MTLS-CA', component: mtls, meta: { section: 'mtls' } },
  { path: '/mtls/clients', name: 'MTLS-CLIENTS', component: mtls, meta: { section: 'mtls' } },
  { path: '/mtls/bindings', name: 'MTLS-BINDINGS', component: mtls, meta: { section: 'mtls' } },
  {
    path: '/lua',
    name: 'Lua',
    component: () => import('./views/LuaView.vue'),
    meta: { section: 'lua' },
  },
  {
    path: '/pages',
    name: 'Pages',
    component: () => import('./views/PagesView.vue'),
    meta: { section: 'pages' },
  },
  {
    path: '/streams',
    name: 'Streams',
    component: () => import('./views/StreamsView.vue'),
    meta: { section: 'streams' },
  },
  {
    path: '/runtime',
    name: 'Runtime',
    component: () => import('./views/RuntimeView.vue'),
    meta: { section: 'runtime' },
  },
  {
    path: '/backups',
    name: 'Backups',
    component: () => import('./views/BackupsView.vue'),
    meta: { section: 'backups' },
  },
  { path: '/:pathMatch(.*)*', redirect: '/sites' },
];
export const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ left: 0, top: 0 }),
});
