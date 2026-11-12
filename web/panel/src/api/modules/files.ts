import http from '../index';
export const getFilesList = (data: any, _node?: string) =>
    http.post<any>('/standalone/files/search', data);
export const createFile = (data: any, _node?: string) =>
    http.post<any>('/standalone/files/create', data);
export const getFileContent = (data: any, _node?: string) =>
    http.post<any>('/standalone/files/content', data);
export const computeDirSize = (data: any, _node?: string) =>
    http.post<any>('/standalone/files/size', data);
export const readByLine = (data: any, _node?: string) => http.post<any>('/standalone/logs', data);
