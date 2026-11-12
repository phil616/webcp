import { ref, computed } from 'vue';
import { expireSession, sessionEpoch } from './auth';
import type { Overview } from './types';
const overview = ref<Overview>();
const busy = ref(false);
const notice = ref<{ text: string; error: boolean }>();
let notificationTimer: ReturnType<typeof setTimeout>;
export function notify(text: string, error = false) {
  notice.value = { text, error };
  clearTimeout(notificationTimer);
  notificationTimer = setTimeout(() => (notice.value = undefined), error ? 15000 : 5000);
}
export async function request<T>(
  url: string,
  method = 'GET',
  body?: unknown,
  expectedRevision?: number,
): Promise<T> {
  const epoch = sessionEpoch.value;
  const response = await fetch('/api' + url, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'If-Match': String(expectedRevision ?? overview.value?.state.revision ?? 0),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 401 && epoch === sessionEpoch.value) expireSession();
  if (epoch !== sessionEpoch.value) throw new Error('登录状态已变化，请重新登录');
  const result = await response.json();
  if (!response.ok) throw new Error(result.error ?? `请求失败 (${response.status})`);
  return result as T;
}
async function refresh() {
  overview.value = await request<Overview>('/overview');
}
async function act(
  fn: () => Promise<unknown>,
  message = '保存成功',
  options: { deploy?: boolean } = {},
) {
  if (busy.value) return false;
  busy.value = true;
  notice.value = undefined;
  try {
    await fn();
    await refresh();
    if (options.deploy && overview.value?.runtime.running) {
      try {
        await request('/runtime/apply', 'POST');
        await refresh();
        notify(`${message}，配置已校验并应用。`);
      } catch (error) {
        notify(
          `配置已保存，但应用失败：${error instanceof Error ? error.message : String(error)}。草稿已保留，可修正后重新应用。`,
          true,
        );
        try {
          await refresh();
        } catch {
          /* The saved draft remains available on reconnect. */
        }
      }
    } else notify(message);
    return true;
  } catch (error) {
    notify(error instanceof Error ? error.message : String(error), true);
    try {
      await refresh();
    } catch {}
    return false;
  } finally {
    busy.value = false;
  }
}
export function useManager() {
  return {
    overview,
    busy,
    notice,
    refresh,
    act,
    state: computed(() => overview.value?.state),
    runtime: computed(() => overview.value?.runtime),
  };
}
export function date(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false });
}
export function size(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

export function clearManagerSession() {
  overview.value = undefined;
  busy.value = false;
  notice.value = undefined;
  clearTimeout(notificationTimer);
}
