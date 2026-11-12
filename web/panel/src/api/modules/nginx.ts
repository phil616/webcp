import http from '@panel/api';
import { useManager } from '@/api';
import { Nginx } from '../interface/nginx';
let baselineRevision: number | undefined;

export const getNginxConfigByScope = async <T = Nginx.NginxParam[]>(req: Nginx.NginxScopeReq) => {
    const version = useManager().state.value?.revision;
    const response = await http.post<T>(`/openresty/scope`, req);
    if (req.scope === 'http-per') baselineRevision = version;
    return response;
};

export const updateNginxConfigByScope = (req: Nginx.NginxConfigReq) => {
    return http.post(`/openresty/update`, req, undefined, { revision: baselineRevision });
};

export const getNginxModules = () => {
    return http.get<Nginx.NginxBuildConfig>(`/openresty/modules`);
};
