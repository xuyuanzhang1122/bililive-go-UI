/*
 * @Author: Jmeow
 * @Date: 2020-01-28 11:25:44
 * @Description: common utils
 */

const API_KEY_STORAGE_KEY = 'bililive_api_key';

function getAPIKey(): string {
    try { return localStorage.getItem(API_KEY_STORAGE_KEY) || ''; } catch { return ''; }
}

function customFetch(arg1: Parameters<typeof fetch>[0], ...args: any[]) {
    // 注入 API Key 鉴权 header（如果已配置）
    const apiKey = getAPIKey();
    const url = typeof arg1 === 'string' ? arg1 : (arg1 instanceof Request ? arg1.url : String(arg1));
    if (apiKey && typeof url === 'string' && url.includes('/api/')) {
        if (args.length > 0 && typeof args[0] === 'object' && args[0] !== null) {
            const init = args[0] as RequestInit;
            const headers = new Headers(init.headers || {});
            if (!headers.has('Authorization')) {
                headers.set('Authorization', `Bearer ${apiKey}`);
            }
            args[0] = { ...init, headers };
        } else if (args.length === 0) {
            args[0] = { headers: new Headers({ 'Authorization': `Bearer ${apiKey}` }) };
        }
    }

    return new Promise((resolve, reject) => {
        fetch.call(null, arg1, ...args)
            .then(async (rsp) => {
                if (rsp.ok) {
                    return rsp.json();
                } else {
                    // Try to parse error message from JSON or text
                    const clonedRsp = rsp.clone();
                    let errMsg = '';
                    try {
                        const data = await rsp.json();
                        errMsg = data.err_msg || data.message || rsp.statusText;
                    } catch (e) {
                        try {
                            errMsg = await clonedRsp.text() || rsp.statusText;
                        } catch (e2) {
                            errMsg = rsp.statusText;
                        }
                    }
                    throw new Error(errMsg);
                }
            })
            .then(data => {
                resolve(data);
            })
            .catch(err => {
                Utils.alertError(err);
                reject(err);
            });
    });
}

// authFetch：原生 fetch 的薄包装，仅注入 Authorization header
// 与 fetch 接口完全兼容（返回 Response，不做 .json() 解包）
// 用于页面级裸 fetch('/api/...') 的鉴权迁移
export function authFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
    const url = typeof input === 'string' ? input : (input instanceof Request ? input.url : input.toString());
    const apiKey = getAPIKey();
    if (apiKey && url.includes('/api/') && !url.includes('/api/auth-status')) {
        const headers = new Headers(init.headers || {});
        if (!headers.has('Authorization')) {
            headers.set('Authorization', `Bearer ${apiKey}`);
        }
        init = { ...init, headers };
    }
    return fetch(input, init);
}

class Utils {
    /**
     * Get request
     * @param url URL
     */
    requestGet(url: string) {
        return customFetch(url);
    }

    /**
     * Post request
     * @param url URL
     * @param body Request body
     */
    requestPost(url: string, body?: object) {
        return customFetch(url, {
            method: 'POST',
            body: JSON.stringify(body),
            headers: new Headers({
                'Content-Type': 'application/json'
            })
        });
    }

    /**
     * Post request
     * @param url URL
     * @param body Request body
     */
    requestPut(url: string, body?: object) {
        return customFetch(url, {
            method: 'PUT',
            body: JSON.stringify(body),
            headers: new Headers({
                'Content-Type': 'application/json'
            })
        })
    }

    /**
     * Delete request
     * @param url URL
     */
    requestDelete(url: string) {
        return customFetch(url, {
            method: 'DELETE'
        });
    }

    /**
     * Delete request with body
     * @param url URL
     * @param body Request body
     */
    requestDeleteWithBody(url: string, body?: object) {
        return customFetch(url, {
            method: 'DELETE',
            body: JSON.stringify(body),
            headers: new Headers({
                'Content-Type': 'application/json'
            })
        });
    }

    /**
     * Patch request
     * @param url URL
     * @param body Request body
     */
    requestPatch(url: string, body?: object) {
        return customFetch(url, {
            method: 'PATCH',
            body: JSON.stringify(body),
            headers: new Headers({
                'Content-Type': 'application/json'
            })
        });
    }

    /**
     * Show Error 
     * @param err error Object
     */
    static alertError(err?: any) {
        console.error(err ? err : "Server Error!");
    }

    static byteSizeToHumanReadableFileSize(size: number): string {
        if (!size) {
            return "0";
        }
        const i = Math.floor(Math.log(size) / Math.log(1024));
        const ret = Number((size / Math.pow(1024, i)).toFixed(2)) + " " + ['B', 'kB', 'MB', 'GB', 'TB'][i];
        return ret;
    }

    static timestampToHumanReadable(timestamp: number): string {
        const date = new Date(timestamp * 1000);
        const year = date.getFullYear().toString().padStart(4, "0");
        const month = (date.getMonth() + 1).toString().padStart(2, "0");
        const day = date.getDate().toString().padStart(2, "0");
        const hour = date.getHours().toString().padStart(2, "0");
        const min = date.getMinutes().toString().padStart(2, "0");
        const sec = date.getSeconds().toString().padStart(2, "0");
        return `${year}-${month}-${day} ${hour}:${min}:${sec}`;
    }
}

export default Utils;
