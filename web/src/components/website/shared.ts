import { ref, toRaw } from 'vue';
import { request, useManager } from '@/api';
import type { CORSConfig, HTTPSConfig, ProxyRule, Site } from '@/types';
export const clone = <T>(value: T): T => structuredClone(toRaw(value));
export const defaultCORS = (): CORSConfig => ({
  enabled: false,
  allowOrigins: '*',
  allowMethods: 'GET,POST,OPTIONS,PUT,DELETE',
  allowHeaders: '',
  allowCredentials: false,
  preflight: true,
});
export const defaultHTTPS = (): HTTPSConfig => ({
  httpConfig: 'HTTPToHTTPS',
  protocols: ['TLSv1.3', 'TLSv1.2'],
  ciphers: '',
  hsts: false,
  hstsIncludeSubDomains: false,
  http2: true,
  http3: false,
});
export const defaultProxy = (): ProxyRule => ({
  id: '',
  name: '',
  enabled: true,
  modifier: '',
  match: '/',
  proxyPass: 'http://127.0.0.1:8080',
  proxyHost: '$host',
  sni: false,
  sslName: '',
  sslVerify: false,
  connectTimeout: 60,
  readTimeout: 60,
  sendTimeout: 60,
  cache: false,
  serverCacheTime: 10,
  serverCacheUnit: 'm',
  cacheTime: 0,
  cacheUnit: 'h',
  replaces: [],
  cors: defaultCORS(),
});
export const lines = (value: string): string[] => value.split(/[\s,]+/).filter(Boolean);
/** Each mounted section owns its edit revision; refreshing overview never overwrites edits. */
export function useWebsiteSection(site: () => Site) {
  const { state, busy, act } = useManager();
  const revision = ref(state.value!.revision);
  function resetRevision() {
    revision.value = state.value!.revision;
  }
  async function save(section: string, body: unknown, method = 'PUT', expected = revision.value) {
    const ok = await act(
      () => request(`/sites/${site().id}/${section}`, method, body, expected),
      '保存成功',
      { deploy: true },
    );
    if (ok) resetRevision();
    return ok;
  }
  return { state, busy, act, revision, resetRevision, save };
}
