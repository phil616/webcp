export function downloadFile(value:string,name='log.txt'){const a=document.createElement('a');a.href=URL.createObjectURL(new Blob([value]));a.download=name;a.click();URL.revokeObjectURL(a.href)}
