import Utils, { getAPIKey } from './common';
import { V2Client, MissingV2Endpoint, Session, Capabilities, DeleteItem, BatchResult } from './generated-api-v2';

export interface RecordingFile {
  name: string; rel_path: string; size: number; mod_time: number;
  recording_id?: string; source_version?: string; size_text?: string;
  file_url?: string; hls_url?: string; thumbnail_url?: string; recording?: boolean;
  playback_status?: 'ready' | 'recording' | 'processing' | 'unsupported';
}
const utils = new Utils();
const path = (value: string) => value.split('/').map(encodeURIComponent).join('/');

export function validateSession(session: Session): Session {
  if (!['ready','processing','paused','failed','cancelled'].includes(session.status)) throw new Error('未知播放状态');
  if (session.status !== 'ready') {
    if (session.url) throw new Error('未就绪会话包含播放地址');
    if (session.status === 'failed' && !session.error) throw new Error('失败会话缺少错误');
    return session;
  }
  if (!session.url || !session.asset_version || !['file','hls'].includes(session.protocol || '') || !session.token_expires_at) throw new Error('播放地址字段无效');
  const url = new URL(session.url, window.location.origin);
  if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/v2/media/') || !url.searchParams.get('play_token') || url.searchParams.has('_key') || url.username || url.password || url.hash) throw new Error('播放地址越界或含管理 Key');
  let refresh = session.refresh_after_seconds;
  // Go omitempty 只在会话硬过期临近时省略零值。
  if (refresh === undefined && session.token_expires_at === session.expires_at && session.expires_at <= Date.now()/1000 + 60) refresh = 0;
  if (refresh === undefined || !Number.isFinite(refresh) || refresh < 0) throw new Error('刷新间隔无效');
  return { ...session, refresh_after_seconds: refresh, url: url.toString() };
}

// 能力由浏览器实际解码查询确认；只列出已测的 SDR H264/AAC LC 范围。
interface CapabilityProbe {
  canPlayType: (type: string) => string;
  decodingInfo: (config: MediaDecodingConfiguration) => Promise<{supported: boolean; smooth: boolean}>;
}
export async function detectCapabilities(probe?: CapabilityProbe): Promise<Capabilities> {
  const element = probe ? null : document.createElement('video');
  const query = probe || { canPlayType: (type: string) => element!.canPlayType(type), decodingInfo: (config: MediaDecodingConfiguration) => navigator.mediaCapabilities.decodingInfo(config) };
  const video: Capabilities['video'] = [];
  for (const [profile, codec] of [['baseline','42E02A'],['main','4D402A'],['high','64002A']]) {
    try {
      const result = await query.decodingInfo({ type: 'file', video: { contentType: `video/mp4; codecs="avc1.${codec}"`, width: 1920, height: 1080, bitrate: 8000000, framerate: 60 } });
      if (result.supported && result.smooth) video.push({ codec: 'h264', profiles: [profile], max_level: 42, max_width: 1920, max_height: 1080, max_fps: 60, max_bit_depth: 8, hdr: false });
    } catch { /* 查询不可用时不声称已支持。 */ }
  }
  const audio: NonNullable<Capabilities['audio']> = [];
  try {
    const result = await query.decodingInfo({ type: 'file', audio: { contentType: 'audio/mp4; codecs="mp4a.40.2"', channels: '2', bitrate: 128000, samplerate: 48000 } });
    if (result.supported && result.smooth) audio.push({ codec: 'aac', profiles: ['lc'], max_channels: 2, max_sample_rate: 48000 });
  } catch { /* 不把缺失查询当成支持。 */ }
  const containers: Capabilities['containers'] = query.canPlayType('video/mp4') ? ['mp4'] : [];
  const protocols: Capabilities['protocols'] = containers.length ? ['file'] : [];
  if (query.canPlayType('application/vnd.apple.mpegurl')) { containers.push('hls'); protocols.push('hls'); }
  return { containers, protocols, video, audio };
}

export class PlaybackAPI {
  readonly client = new V2Client((input,init={}) => {
    const headers=new Headers(init.headers);
    if(this.key) headers.set('Authorization',`Bearer ${this.key}`);
    return fetch(input,{...init,headers});
  });
  mode: 'auto' | 'v2' | 'legacy' = 'auto';
  private key = getAPIKey();
  private origin = window.location.origin;
  isCurrent(): boolean { return this.key === getAPIKey() && this.origin === window.location.origin; }
  assertCurrent(): void { if (!this.isCurrent()) throw new Error('服务器或认证已变化，请重新打开'); }
  async loadHistory(videoPath: string): Promise<{recording_id?:string;source_version?:string;position_seconds:number;duration_seconds:number} | null> {
    this.assertCurrent();const headers=new Headers();if(this.key) headers.set('Authorization',`Bearer ${this.key}`);
    const path=videoPath.split('/').map(encodeURIComponent).join('/');
    const response=await fetch(new URL('/api/history/'+path,this.origin),{headers,redirect:'error'});
    this.assertCurrent();if(response.status===404) return null;
    if(!response.ok) throw new Error('读取观看历史失败');
    const payload=await response.json();if(payload.err_no!==undefined && payload.err_no!==0) throw new Error(payload.err_msg || '读取观看历史失败');
    return 'data' in payload?payload.data:payload;
  }
  async saveHistory(videoPath: string, videoName: string, positionSeconds: number, durationSeconds: number, identity?: {recording_id:string;source_version:string}): Promise<void> {
    this.assertCurrent();
    const headers = new Headers({'Content-Type':'application/json'});
    if(this.key) headers.set('Authorization',`Bearer ${this.key}`);
    const response = await fetch(new URL('/api/history',this.origin),{method:'POST',headers,redirect:'error',body:JSON.stringify({...identity,video_path:videoPath,video_name:videoName,position_seconds:positionSeconds,duration_seconds:durationSeconds})});
    if(!response.ok) throw new Error('观看历史同步失败');
    const result = await response.json();
    if(result.err_no !== 0) throw new Error(result.err_msg || '观看历史同步失败');
    this.assertCurrent();
  }
  async list(folder: string): Promise<RecordingFile[]> {
    this.assertCurrent();
    if (this.mode === 'legacy') return this.legacyList(folder);
    try {
      const files: RecordingFile[] = [], ids = new Set<string>(), cursors = new Set<string>();
      let after = '';
      do {
        const {data} = await this.client.getV2Recordings({ query: { after, limit: 100 } }); this.assertCurrent();
        if (!data) throw new Error('清单缺少 data');
        for (const item of data.items) {
          if (ids.has(item.recording_id)) throw new Error('清单身份重复'); ids.add(item.recording_id);
          if (item.room === folder) files.push({ name: item.path.split('/').pop() || item.path, rel_path: item.path, size: item.size_bytes, size_text: item.size_text, mod_time: 0, recording_id: item.recording_id, source_version: item.source_version });
        }
        after = data.next_cursor;
        if (after && cursors.has(after)) throw new Error('清单游标循环'); cursors.add(after);
      } while (after);
      this.mode = 'v2';
      return files;
    } catch (error) {
      if (this.mode === 'auto' && error instanceof MissingV2Endpoint) { this.mode = 'legacy'; return this.legacyList(folder); }
      throw error;
    }
  }
  private async legacyList(folder: string): Promise<RecordingFile[]> {
    const data = await utils.requestGet(`/api/video-files/${path(folder)}`);
    if (!Array.isArray(data)) throw new Error('旧视频列表格式无效');
    return data;
  }
  async delete(items: DeleteItem[]): Promise<BatchResult> {
    this.assertCurrent();
    if (!items.length || items.length > 100 || new Set(items.map(i=>i.recording_id)).size !== items.length) throw new Error('删除身份无效');
    const {data} = await this.client.batchDeleteV2Recordings({body:{items}});this.assertCurrent();
    if (!data || data.items.length !== items.length || new Set(data.items.map(i=>i.recording_id)).size !== items.length || data.items.some(i=>!items.some(r=>r.recording_id===i.recording_id))) throw new Error('批删结果身份不匹配');
    return data;
  }
}

// 旧 resolver 的两种已知线形有独立入口，不用 data ?? response 隐藏坏封装。
export function legacyResolve(value: any, baseURL = window.location.origin, apiKey = getAPIKey()): { status: string; url?: string; protocol?: string; retry_after_seconds?: number; error?: string } {
  const envelope = value && typeof value === 'object' && ('data' in value || 'err_no' in value);
  if (envelope && (value.error || value.err_no || !value.data)) throw new Error(value.error?.message || value.err_msg || '旧播放封装错误');
  const result = envelope ? value.data : value;
  if (!result || !['ready','processing','failed','recording'].includes(result.status)) throw new Error('旧播放状态无效');
  if (result.status === 'ready') {
    if (typeof result.url !== 'string' || !result.url.trim()) throw new Error('旧播放缺少地址');
    const base=new URL(baseURL.endsWith('/')?baseURL:baseURL+'/');
    const url=new URL(result.url.startsWith('/')?base.toString().replace(/\/$/,'')+result.url:result.url,base);
    if (!['http:','https:'].includes(url.protocol) || url.username || url.password) throw new Error('旧播放地址无效');
    const signed=(url.searchParams.has('sig')&&url.searchParams.has('expires'))||url.searchParams.has('play_token');
    if(url.origin!==base.origin)url.searchParams.delete('_key');
    else if(!signed&&apiKey&&!url.searchParams.has('_key'))url.searchParams.set('_key',apiKey);
    return {...result,url:url.toString()};
  }
  if (result.url) throw new Error('未就绪旧播放包含地址');
  if (result.status === 'processing') {
    const raw=result.retry_after_seconds;
    return {...result,retry_after_seconds:typeof raw==='number'&&Number.isFinite(raw)?Math.min(10,Math.max(1,raw)):2};
  }
  return result;
}
