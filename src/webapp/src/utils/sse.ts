import { authFetch } from './common';

export type SSEEventType = 'live_update' | 'log' | 'conn_stats' | 'recorder_status' | 'connected' | 'list_change' | 'rate_limit_update' | 'update_available' | 'update_downloading' | 'update_ready' | 'update_error' | 'pipeline_task_update';
export interface SSEMessage { type: SSEEventType; room_id: string; data: any; }
export type SSECallback = (message: SSEMessage) => void;
interface Subscription { roomId: string; eventType: SSEEventType | '*'; callback: SSECallback; }

// 保留跨网络块的 UTF8 与行边界，空行才提交完整事件。
export class SSEDecoder {
  private decoder = new TextDecoder();
  private pending = '';
  private type = 'message';
  private data: string[] = [];
  constructor(private emit: (type: string, data: string) => void) {}
  push(bytes: Uint8Array): void {
    this.pending += this.decoder.decode(bytes, { stream: true });
    if (this.pending.length > 1024 * 1024) throw new Error('SSE 事件过大');
    let match: RegExpExecArray | null;
    while ((match = /\r\n|\r|\n/.exec(this.pending))) {
      if (match[0] === '\r' && match.index === this.pending.length - 1) break;
      const line = this.pending.slice(0, match.index);
      this.pending = this.pending.slice(match.index + match[0].length);
      if (!line) {
        if (this.data.length) this.emit(this.type, this.data.join('\n'));
        this.type = 'message'; this.data = [];
      } else if (!line.startsWith(':')) {
        const colon = line.indexOf(':');
        const field = colon < 0 ? line : line.slice(0, colon);
        const value = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '');
        if (field === 'event') this.type = value;
        if (field === 'data') {
          this.data.push(value);
          if (this.data.reduce((n, v) => n + v.length, 0) > 1024 * 1024) throw new Error('SSE 事件过大');
        }
      }
    }
  }
}

export class SSEManager {
  private subscriptions = new Map<string, Subscription>();
  private controller: AbortController | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private connected = false;
  private generation = 0;
  private attempts = 0;
  private sequence = 0;

  private async connect(): Promise<void> {
    if (this.controller || !this.subscriptions.size) return;
    const controller = new AbortController(), generation = this.generation;
    this.controller = controller;
    let retry = true;
    try {
      const response = await authFetch(`${window.location.origin}/api/sse`, { signal: controller.signal, headers: { Accept: 'text/event-stream' }, cache: 'no-store', redirect: 'error' });
      if (!response.ok) {
        if (response.status === 401 || response.status === 403) retry = false;
        throw new Error(`SSE HTTP ${response.status}`);
      }
      if (!response.body || !response.headers.get('Content-Type')?.startsWith('text/event-stream')) { retry = false; throw new Error('SSE 响应格式无效'); }
      if (generation !== this.generation || controller.signal.aborted) return;
      this.connected = true;
      const decoder = new SSEDecoder((type, data) => {
        if (generation !== this.generation || controller.signal.aborted) return;
        try {
          const message = JSON.parse(data) as SSEMessage;
          message.type = type as SSEEventType;
          this.attempts = 0;
          this.subscriptions.forEach(sub => {
            if ((sub.roomId === '*' || sub.roomId === message.room_id) && (sub.eventType === '*' || sub.eventType === type)) {
              try { sub.callback(message); } catch (e) { console.error('SSE 回调失败', e); }
            }
          });
        } catch (e) { console.error('SSE 事件无效', e); }
      });
      const reader = response.body.getReader();
      const cancel = () => { void reader.cancel().catch(() => {}); };
      controller.signal.addEventListener('abort', cancel, { once: true });
      try {
        while (!controller.signal.aborted) {
          const { value, done } = await reader.read();
          if (done) break;
          decoder.push(value);
        }
      } finally { controller.signal.removeEventListener('abort', cancel); await reader.cancel().catch(() => {}); reader.releaseLock(); }
    } catch (e) { if (!controller.signal.aborted) console.error('SSE 连接失败', e); }
    finally {
      if (this.controller === controller) { this.controller = null; this.connected = false; }
      if (retry && generation === this.generation && !controller.signal.aborted && this.subscriptions.size && this.attempts < 10) {
        const delay = Math.min(30000, 1000 * 2 ** this.attempts++);
        this.timer = setTimeout(() => { this.timer = null; void this.connect(); }, delay);
      }
    }
  }
  private stop(): void {
    this.generation++; this.controller?.abort(); this.controller = null; this.connected = false;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null; this.attempts = 0;
  }
  subscribe(roomId: string, eventType: SSEEventType | '*', callback: SSECallback): string {
    const id = String(++this.sequence);
    this.subscriptions.set(id, { roomId, eventType, callback });
    if (this.subscriptions.size === 1) void this.connect();
    return id;
  }
  unsubscribe(id: string): void { this.subscriptions.delete(id); if (!this.subscriptions.size) this.stop(); }
  isConnected(): boolean { return this.connected; }
  getSubscriberCount(): number { return this.subscriptions.size; }
  reconnect(): void { this.stop(); if (this.subscriptions.size) void this.connect(); }
}
export const sseManager = new SSEManager();
export const subscribeSSE = (roomId: string, eventType: SSEEventType | '*', callback: SSECallback): string => sseManager.subscribe(roomId, eventType, callback);
export const unsubscribeSSE = (id: string): void => sseManager.unsubscribe(id);
