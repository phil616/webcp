import { ResultEnum } from '@panel/enums/http-enum';
export type RequestConfig = {
    signal?: AbortSignal;
    timeout?: number;
    revision?: number;
    [key: string]: unknown;
};
export type TransportOptions = { timeout: number; signal?: AbortSignal; revision?: number };
type Bridge = (
    path: string,
    method: string,
    data: unknown,
    blob: boolean,
    options: TransportOptions,
) => Promise<unknown>;
let bridge: Bridge;
export function setBridge(value: Bridge) {
    bridge = value;
}
function options(timeout: number, config?: RequestConfig): TransportOptions {
    return { timeout, signal: config?.signal, revision: config?.revision };
}
const call = async <T>(
    path: string,
    method: string,
    data: unknown,
    timeout: number = Number(ResultEnum.TIMEOUT),
    config?: RequestConfig,
): Promise<{ data: T }> => ({
    data: (await bridge(path, method, data, false, options(timeout, config))) as T,
});
export default {
    get: <T>(path: string, data?: unknown) => call<T>(path, 'GET', data),
    post: <T>(
        path: string,
        data?: unknown,
        timeout: number = Number(ResultEnum.TIMEOUT),
        config?: RequestConfig,
    ) => call<T>(path, 'POST', data, timeout, config),
    postLocalNode: <T>(
        path: string,
        data?: unknown,
        timeout: number = Number(ResultEnum.TIMEOUT),
        config?: RequestConfig,
    ) => call<T>(path, 'POST', data, timeout, config),
    upload: <T>(path: string, data: FormData, config?: RequestConfig) =>
        call<T>(path, 'POST', data, config?.timeout ?? 600000, config),
    download: async <T = Blob>(
        path: string,
        data: unknown,
        config?: RequestConfig,
    ): Promise<Blob> =>
        (await bridge(
            path,
            'POST',
            data,
            true,
            options(config?.timeout ?? Number(ResultEnum.TIMEOUT), config),
        )) as Blob,
};
