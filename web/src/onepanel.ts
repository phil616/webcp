import { router } from './router';
import { expireSession, sessionEpoch } from './auth';
import { setBridge } from '@panel/entry';
import { useManager, notify } from './api';
setBridge(
  async (
    path: string,
    method: string,
    data: unknown,
    blob: boolean,
    options: { timeout: number; signal?: AbortSignal; revision?: number },
  ) => {
    const { state, refresh } = useManager();
    if (router.currentRoute.value.path.startsWith('/mtls') && path.startsWith('/websites/ca'))
      path = path.replace('/websites/ca', '/mtls/ca');
    try {
      const multipart = data instanceof FormData;
      const epoch = sessionEpoch.value;
      const response = await fetch('/api/panel' + path, {
        method,
        signal: options.signal ?? AbortSignal.timeout(options.timeout),
        headers: {
          ...(!multipart ? { 'Content-Type': 'application/json' } : {}),
          'If-Match': String(options.revision ?? state.value?.revision ?? 0),
        },
        body:
          data === undefined ? undefined : multipart ? (data as FormData) : JSON.stringify(data),
      });
      if (response.status === 401 && epoch === sessionEpoch.value) expireSession();
      if (epoch !== sessionEpoch.value) throw new Error('登录状态已变化，请重新登录');
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error ?? `请求失败 (${response.status})`);
      }
      const result = blob ? await response.blob() : await response.json();
      if (
        method !== 'GET' &&
        !['/search', '/list', '/resolve', '/download'].some((s) => path.endsWith(s)) &&
        !path.startsWith('/standalone/')
      )
        await refresh();
      return result;
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), true);
      throw error;
    }
  },
);
