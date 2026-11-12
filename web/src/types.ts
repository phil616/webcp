export interface DomainBinding {
  domain: string;
  port: number;
  ssl: boolean;
}
export interface HTTPSConfig {
  httpConfig: 'HTTPToHTTPS' | 'HTTPAlso' | 'HTTPSOnly';
  protocols: string[];
  ciphers: string;
  hsts: boolean;
  hstsIncludeSubDomains: boolean;
  http2: boolean;
  http3: boolean;
}
export interface CORSConfig {
  enabled: boolean;
  allowOrigins: string;
  allowMethods: string;
  allowHeaders: string;
  allowCredentials: boolean;
  preflight: boolean;
}
export interface RealIPConfig {
  enabled: boolean;
  from: string[];
  header: string;
}
export interface Replacement {
  from: string;
  to: string;
}
export interface ProxyRule {
  id: string;
  name: string;
  enabled: boolean;
  modifier: '' | '=' | '^~' | '~' | '~*';
  match: string;
  proxyPass: string;
  proxyHost: string;
  sni: boolean;
  sslName: string;
  sslVerify: boolean;
  connectTimeout: number;
  readTimeout: number;
  sendTimeout: number;
  cache: boolean;
  serverCacheTime: number;
  serverCacheUnit: string;
  cacheTime: number;
  cacheUnit: string;
  replaces: Replacement[];
  cors: CORSConfig;
}
export interface RedirectRule {
  id: string;
  name: string;
  enabled: boolean;
  type: 'domain' | 'path' | '404';
  domains: string[];
  path: string;
  redirect: number;
  target: string;
  keepPath: boolean;
  redirectRoot: boolean;
}
export interface StreamProxy {
  id: string;
  name: string;
  protocol: 'tcp' | 'udp';
  listenAddress: string;
  listenPort: number;
  targetHost: string;
  targetPort: number;
  connectTimeout: number;
  proxyTimeout: number;
  udpResponses: number;
  proxyProtocol: boolean;
  enabled: boolean;
  createdAt: string;
}
export interface PortBinding {
  hostAddress: string;
  hostPort: number;
  containerPort: number;
  protocol: 'tcp' | 'udp';
}
export interface Site {
  headers: ResponseHeader[];
  security: { followSymlinks: boolean; allowHiddenFiles: boolean };
  panelId?: number;
  proxyCache: { memoryMb: number; maxSizeMb: number; inactiveMinutes: number };
  bindings: DomainBinding[];
  runDirectory: string;
  index: string[];
  https: HTTPSConfig;
  cors: CORSConfig;
  realIp: RealIPConfig;
  proxies: ProxyRule[];
  redirects: RedirectRule[];
  id: string;
  name: string;
  domains: string[];
  kind: 'static' | 'proxy';
  upstream: string;
  enabled: boolean;
  tlsId: string;
  luaBindings: Record<string, string>;
  createdAt: string;
}
export interface ResponseHeader {
  scope: string;
  tls: boolean;
  name: string;
  value: string;
  always: boolean;
}
export interface Lua {
  id: string;
  name: string;
  kind: 'module' | 'block';
  description: string;
  code: string;
}
export interface Certificate {
  id: string;
  name: string;
  kind: 'ca' | 'server' | 'client';
  caId: string;
  serial: string;
  domains: string[];
  certPem: string;
  notBefore: string;
  notAfter: string;
  revokedAt?: string;
}
export interface S3 {
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  accessKey: string;
  secretKey?: string;
  pathStyle: boolean;
}
export interface State {
  openresty?: { http: Record<string, string> };
  schema: number;
  revision: number;
  sites: Site[];
  streams: StreamProxy[];
  lua: Lua[];
  certificates: Certificate[];
  mtls: {
    authorities: Certificate[];
    clients: Certificate[];
    bindings: { siteId: string; caId: string; enabled: boolean }[];
  };
  s3: S3;
}
export interface Runtime {
  available: boolean;
  running: boolean;
  container: string;
  image: string;
  appliedRevision: number;
  publishedPorts: PortBinding[];
  detail: string;
}
export interface Overview {
  state: State;
  runtime: Runtime;
}
export interface Backup {
  id: string;
  createdAt: string;
  size: number;
  sha256: string;
  revision: number;
  objectKey?: string;
  remote: boolean;
}
export interface SiteFile {
  path: string;
  size: number;
  modifiedAt: string;
}

export interface PageDocument {
  pages: Record<string, string>;
  etag: string;
}
