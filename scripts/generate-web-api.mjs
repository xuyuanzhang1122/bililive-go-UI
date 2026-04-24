import fs from 'node:fs';
import path from 'node:path';

const repoRoot = process.cwd();
const serverPath = path.join(repoRoot, 'src', 'servers', 'server.go');
const outputPath = path.join(repoRoot, 'src', 'webapp', 'src', 'utils', 'generated-api.ts');

const source = fs.readFileSync(serverPath, 'utf8');
const routePattern = /apiRoute\.HandleFunc\("([^"]+)",\s*([A-Za-z0-9_]+)\)\.Methods\(([^)]+)\)/g;

const routes = [];
let match;
while ((match = routePattern.exec(source)) !== null) {
  const [, pathTemplate, handlerName, methodsRaw] = match;
  const methods = [...methodsRaw.matchAll(/"([^"]+)"/g)].map((m) => m[1]);
  for (const method of methods) {
    routes.push({
      key: uniqueKey(routes, `${handlerName}_${method.toLowerCase()}`),
      method,
      path: `/api${pathTemplate}`,
      handler: handlerName,
      params: extractParams(pathTemplate),
    });
  }
}

routes.sort((a, b) => a.key.localeCompare(b.key));

const endpointEntries = routes
  .map((route) => {
    const params = route.params.map((p) => `'${p}'`).join(', ');
    return `  ${route.key}: { method: '${route.method}', path: '${route.path}', handler: '${route.handler}', params: [${params}] },`;
  })
  .join('\n');

const content = `// 此文件由 scripts/generate-web-api.mjs 自动生成，请不要手工修改。

export const apiEndpoints = {
${endpointEntries}
} as const;

export type ApiEndpointName = keyof typeof apiEndpoints;
export type ApiEndpoint = typeof apiEndpoints[ApiEndpointName];
export type ApiQuery = Record<string, string | number | boolean | null | undefined>;
export type ApiPathParams = Record<string, string | number>;

export interface ApiRequestOptions {
  params?: ApiPathParams;
  query?: ApiQuery;
  body?: unknown;
  headers?: HeadersInit;
}

export async function requestApi<T = unknown>(name: ApiEndpointName, options: ApiRequestOptions = {}): Promise<T> {
  const endpoint = apiEndpoints[name];
  const url = buildApiURL(endpoint.path, options.params, options.query);
  const init: RequestInit = {
    method: endpoint.method,
    headers: buildHeaders(options.headers, options.body !== undefined),
  };
  if (options.body !== undefined) {
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(url, init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
  return response.json() as Promise<T>;
}

function buildHeaders(headers: HeadersInit | undefined, hasBody: boolean): HeadersInit | undefined {
  if (!hasBody) {
    return headers;
  }
  const merged = new Headers(headers);
  if (!merged.has('Content-Type')) {
    merged.set('Content-Type', 'application/json');
  }
  return merged;
}

function buildApiURL(pathTemplate: string, params: ApiPathParams = {}, query: ApiQuery = {}): string {
  let url = pathTemplate.replace(/\\{([^}:]+)(?::([^}]+))?\\}/g, (_match, name: string, pattern: string | undefined) => {
    const value = params[name];
    if (value === undefined || value === null) {
      throw new Error(\`缺少路径参数: \${name}\`);
    }
    if (pattern === '.*') {
      return String(value).split('/').map(encodeURIComponent).join('/');
    }
    return encodeURIComponent(String(value));
  });
  const search = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      search.append(key, String(value));
    }
  });
  const queryString = search.toString();
  if (queryString) {
    url += \`?\${queryString}\`;
  }
  return url;
}

async function readErrorMessage(response: Response): Promise<string> {
  const cloned = response.clone();
  try {
    const data = await response.json();
    return data.err_msg || data.message || response.statusText;
  } catch (_) {
    return (await cloned.text()) || response.statusText;
  }
}
`;

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, content);
console.log(`已生成 ${path.relative(repoRoot, outputPath)}，共 ${routes.length} 个 API 方法`);

function extractParams(pathTemplate) {
  return [...pathTemplate.matchAll(/\{([^}:]+)(?::[^}]+)?\}/g)].map((m) => m[1]);
}

function uniqueKey(existing, base) {
  let key = base;
  let index = 2;
  const used = new Set(existing.map((route) => route.key));
  while (used.has(key)) {
    key = `${base}_${index}`;
    index += 1;
  }
  return key;
}
