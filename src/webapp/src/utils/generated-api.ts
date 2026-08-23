// 此文件由 scripts/generate-web-api.mjs 自动生成，请不要手工修改。

export const apiEndpoints = {
  addLives_post: { method: 'POST', path: '/api/lives', handler: 'addLives', params: [] },
  applyUpdate_post: { method: 'POST', path: '/api/update/apply', handler: 'applyUpdate', params: [] },
  batchDeleteFiles_post: { method: 'POST', path: '/api/batch/file/delete', handler: 'batchDeleteFiles', params: [] },
  batchRenameFiles_put: { method: 'PUT', path: '/api/batch/file/rename', handler: 'batchRenameFiles', params: [] },
  cancelUpdate_post: { method: 'POST', path: '/api/update/cancel', handler: 'cancelUpdate', params: [] },
  checkOpenListStorageHealth_get: { method: 'GET', path: '/api/openlist/check-storage', handler: 'checkOpenListStorageHealth', params: [] },
  checkRemoteWebuiUpdate_get: { method: 'GET', path: '/api/webui/remote/check', handler: 'checkRemoteWebuiUpdate', params: [] },
  checkUpdate_get: { method: 'GET', path: '/api/update/check', handler: 'checkUpdate', params: [] },
  createAPIKeyUser_post: { method: 'POST', path: '/api/api-keys', handler: 'createAPIKeyUser', params: [] },
  createBackup_post: { method: 'POST', path: '/api/backups', handler: 'createBackup', params: [] },
  createSignedURL_get: { method: 'GET', path: '/api/signed-url', handler: 'createSignedURL', params: [] },
  deleteAPIKeyUser_delete: { method: 'DELETE', path: '/api/api-keys/{id}', handler: 'deleteAPIKeyUser', params: ['id'] },
  deleteFile_delete: { method: 'DELETE', path: '/api/file/{path:.*}', handler: 'deleteFile', params: ['path'] },
  deletePlatformConfig_delete: { method: 'DELETE', path: '/api/config/platforms/{platform}', handler: 'deletePlatformConfig', params: ['platform'] },
  deleteWatchHistory_delete: { method: 'DELETE', path: '/api/history/{videoPath:.*}', handler: 'deleteWatchHistory', params: ['videoPath'] },
  doRollback_post: { method: 'POST', path: '/api/update/rollback', handler: 'doRollback', params: [] },
  downloadUpdate_post: { method: 'POST', path: '/api/update/download', handler: 'downloadUpdate', params: [] },
  getAllLives_get: { method: 'GET', path: '/api/lives', handler: 'getAllLives', params: [] },
  getAPIKeyUsers_get: { method: 'GET', path: '/api/api-keys', handler: 'getAPIKeyUsers', params: [] },
  getAuthMe_get: { method: 'GET', path: '/api/auth/me', handler: 'getAuthMe', params: [] },
  getAuthStatus_get: { method: 'GET', path: '/api/auth-status', handler: 'getAuthStatus', params: [] },
  getBackup_get: { method: 'GET', path: '/api/backups/{id}', handler: 'getBackup', params: ['id'] },
  getBilibiliQRCode_get: { method: 'GET', path: '/api/bilibili/qrcode', handler: 'getBilibiliQRCode', params: [] },
  getCleanupCandidates_get: { method: 'GET', path: '/api/cleanup-candidates', handler: 'getCleanupCandidates', params: [] },
  getConfig_get: { method: 'GET', path: '/api/config', handler: 'getConfig', params: [] },
  getConfigSyncStatus_get: { method: 'GET', path: '/api/config/sync-status', handler: 'getConfigSyncStatus', params: [] },
  getDiskDevices_get: { method: 'GET', path: '/api/iostats/devices', handler: 'getDiskDevices', params: [] },
  getDiskIOStats_get: { method: 'GET', path: '/api/iostats/disk', handler: 'getDiskIOStats', params: [] },
  getDouyinCookieConfig_get: { method: 'GET', path: '/api/config/douyin-cookie', handler: 'getDouyinCookieConfig', params: [] },
  getEffectiveConfig_get: { method: 'GET', path: '/api/config/effective', handler: 'getEffectiveConfig', params: [] },
  getFileInfo_get: { method: 'GET', path: '/api/file/{path:.*}', handler: 'getFileInfo', params: ['path'] },
  getHeadlessBrowserConfig_get: { method: 'GET', path: '/api/config/headless-browser', handler: 'getHeadlessBrowserConfig', params: [] },
  getHLSPlaylist_get: { method: 'GET', path: '/api/stream/hls/{path:.*}', handler: 'getHLSPlaylist', params: ['path'] },
  getHLSSegment_get: { method: 'GET', path: '/api/stream/hls-segment/{cache_key}/{segment}', handler: 'getHLSSegment', params: ['cache_key', 'segment'] },
  getInfo_get: { method: 'GET', path: '/api/info', handler: 'getInfo', params: [] },
  getIOStats_get: { method: 'GET', path: '/api/iostats', handler: 'getIOStats', params: [] },
  getIOStatsFilters_get: { method: 'GET', path: '/api/iostats/filters', handler: 'getIOStatsFilters', params: [] },
  getLatestRelease_get: { method: 'GET', path: '/api/update/latest', handler: 'getLatestRelease', params: [] },
  getLauncherStatus_get: { method: 'GET', path: '/api/update/launcher', handler: 'getLauncherStatus', params: [] },
  getLive_get: { method: 'GET', path: '/api/lives/{id}', handler: 'getLive', params: ['id'] },
  getLiveHistory_get: { method: 'GET', path: '/api/lives/{id}/history', handler: 'getLiveHistory', params: ['id'] },
  getLiveHostCookie_get: { method: 'GET', path: '/api/cookies', handler: 'getLiveHostCookie', params: [] },
  getLiveLogs_get: { method: 'GET', path: '/api/lives/{id}/logs', handler: 'getLiveLogs', params: ['id'] },
  getLiveNameHistory_get: { method: 'GET', path: '/api/lives/{id}/name-history', handler: 'getLiveNameHistory', params: ['id'] },
  getLiveSessionHistory_get: { method: 'GET', path: '/api/lives/{id}/sessions', handler: 'getLiveSessionHistory', params: ['id'] },
  getMemoryCategories_get: { method: 'GET', path: '/api/iostats/memory/categories', handler: 'getMemoryCategories', params: [] },
  getMemoryStats_get: { method: 'GET', path: '/api/memory', handler: 'getMemoryStats', params: [] },
  getMemoryStatsHistory_get: { method: 'GET', path: '/api/iostats/memory', handler: 'getMemoryStatsHistory', params: [] },
  getOpenListStatus_get: { method: 'GET', path: '/api/openlist/status', handler: 'getOpenListStatus', params: [] },
  getPlatformStats_get: { method: 'GET', path: '/api/config/platforms', handler: 'getPlatformStats', params: [] },
  getRawConfig_get: { method: 'GET', path: '/api/raw-config', handler: 'getRawConfig', params: [] },
  getRemoteWebuiStatus_get: { method: 'GET', path: '/api/webui/remote/status', handler: 'getRemoteWebuiStatus', params: [] },
  getRequestStatus_get: { method: 'GET', path: '/api/iostats/requests', handler: 'getRequestStatus', params: [] },
  getRestoreStatus_get: { method: 'GET', path: '/api/backups/restore/status/{job_id}', handler: 'getRestoreStatus', params: ['job_id'] },
  getRollbackInfo_get: { method: 'GET', path: '/api/update/rollback', handler: 'getRollbackInfo', params: [] },
  getThumbnail_get: { method: 'GET', path: '/api/thumbnail/{path:.*}', handler: 'getThumbnail', params: ['path'] },
  getUpdateStatus_get: { method: 'GET', path: '/api/update/status', handler: 'getUpdateStatus', params: [] },
  getVideoFiles_get: { method: 'GET', path: '/api/video-files/{path:.*}', handler: 'getVideoFiles', params: ['path'] },
  getVideoLibrary_get: { method: 'GET', path: '/api/video-library', handler: 'getVideoLibrary', params: [] },
  getWatchHistory_get: { method: 'GET', path: '/api/history', handler: 'getWatchHistory', params: [] },
  getWatchHistoryItem_get: { method: 'GET', path: '/api/history/{videoPath:.*}', handler: 'getWatchHistoryItem', params: ['videoPath'] },
  localDoctor_post: { method: 'POST', path: '/api/local/doctor', handler: 'localDoctor', params: [] },
  localRestart_post: { method: 'POST', path: '/api/local/restart', handler: 'localRestart', params: [] },
  parseLiveAction_get: { method: 'GET', path: '/api/lives/{id}/{action}', handler: 'parseLiveAction', params: ['id', 'action'] },
  pollBilibiliQRCode_get: { method: 'GET', path: '/api/bilibili/qrcode/poll', handler: 'pollBilibiliQRCode', params: [] },
  postCleanupAction_post: { method: 'POST', path: '/api/cleanup-candidates/action', handler: 'postCleanupAction', params: [] },
  previewOutputTmpl_post: { method: 'POST', path: '/api/config/preview-template', handler: 'previewOutputTmpl', params: [] },
  probeHeadlessBrowser_post: { method: 'POST', path: '/api/tools/headless-browser/probe', handler: 'probeHeadlessBrowser', params: [] },
  putConfig_put: { method: 'PUT', path: '/api/config', handler: 'putConfig', params: [] },
  putDouyinCookieConfig_put: { method: 'PUT', path: '/api/config/douyin-cookie', handler: 'putDouyinCookieConfig', params: [] },
  putLiveHostCookie_put: { method: 'PUT', path: '/api/cookies', handler: 'putLiveHostCookie', params: [] },
  putRawConfig_put: { method: 'PUT', path: '/api/raw-config', handler: 'putRawConfig', params: [] },
  removeLive_delete: { method: 'DELETE', path: '/api/lives/{id}', handler: 'removeLive', params: ['id'] },
  renameFile_put: { method: 'PUT', path: '/api/file/{path:.*}', handler: 'renameFile', params: ['path'] },
  resolveUrl_get: { method: 'GET', path: '/api/resolve-url', handler: 'resolveUrl', params: [] },
  restoreBackup_post: { method: 'POST', path: '/api/backups/restore', handler: 'restoreBackup', params: [] },
  setUpdateChannel_put: { method: 'PUT', path: '/api/update/channel', handler: 'setUpdateChannel', params: [] },
  sseHandler_get: { method: 'GET', path: '/api/sse', handler: 'sseHandler', params: [] },
  switchStreamHandler_post: { method: 'POST', path: '/api/lives/{id}/switchStream', handler: 'switchStreamHandler', params: ['id'] },
  updateAPIKeyUser_patch: { method: 'PATCH', path: '/api/api-keys/{id}', handler: 'updateAPIKeyUser', params: ['id'] },
  updateConfig_patch: { method: 'PATCH', path: '/api/config', handler: 'updateConfig', params: [] },
  updateHeadlessBrowserConfig_patch: { method: 'PATCH', path: '/api/config/headless-browser', handler: 'updateHeadlessBrowserConfig', params: [] },
  updatePlatformConfig_patch: { method: 'PATCH', path: '/api/config/platforms/{platform}', handler: 'updatePlatformConfig', params: ['platform'] },
  updatePlatformConfig_put: { method: 'PUT', path: '/api/config/platforms/{platform}', handler: 'updatePlatformConfig', params: ['platform'] },
  updateRoomConfig_patch: { method: 'PATCH', path: '/api/config/rooms/{url:.*}', handler: 'updateRoomConfig', params: ['url'] },
  updateRoomConfig_put: { method: 'PUT', path: '/api/config/rooms/{url:.*}', handler: 'updateRoomConfig', params: ['url'] },
  updateRoomConfigById_patch: { method: 'PATCH', path: '/api/config/rooms/id/{id}', handler: 'updateRoomConfigById', params: ['id'] },
  updateRoomConfigById_put: { method: 'PUT', path: '/api/config/rooms/id/{id}', handler: 'updateRoomConfigById', params: ['id'] },
  upsertWatchHistory_post: { method: 'POST', path: '/api/history', handler: 'upsertWatchHistory', params: [] },
  verifyBilibiliCookie_post: { method: 'POST', path: '/api/bilibili/cookie/verify', handler: 'verifyBilibiliCookie', params: [] },
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
  let url = pathTemplate.replace(/\{([^}:]+)(?::([^}]+))?\}/g, (_match, name: string, pattern: string | undefined) => {
    const value = params[name];
    if (value === undefined || value === null) {
      throw new Error(`缺少路径参数: ${name}`);
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
    url += `?${queryString}`;
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
