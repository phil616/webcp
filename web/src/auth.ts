import { ref } from 'vue';
export const authenticated = ref(false);
export const checkingSession = ref(true);
export const sessionEpoch = ref(0);
export function expireSession() {
  authenticated.value = false;
  sessionEpoch.value++;
}
async function authRequest(path: string, body?: unknown) {
  const response = await fetch('/api/auth/' + path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error ?? '登录请求失败');
  return result;
}
export async function checkSession() {
  try {
    const result = await authRequest('status');
    authenticated.value = result.authenticated;
  } finally {
    checkingSession.value = false;
  }
}
export async function login(key: string) {
  await authRequest('login', { key });
  sessionEpoch.value++;
  authenticated.value = true;
}
export async function logout() {
  await authRequest('logout', {});
  expireSession();
}
