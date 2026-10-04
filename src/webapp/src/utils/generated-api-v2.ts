// 由 T5 OpenAPI 自动生成，请运行 scripts/generate-web-api.mjs。
import { authFetch } from './common';

export type Error = { "code": string; "message": string; "retryable": boolean; "retry_after_seconds"?: number };
export type VideoCapability = { "codec": string; "profiles": Array<string>; "max_level": number; "max_width": number; "max_height": number; "max_fps": number; "max_bit_depth": number; "hdr"?: boolean };
export type AudioCapability = { "codec": string; "profiles": Array<string>; "max_channels": number; "max_sample_rate": number };
export type Capabilities = { "containers": Array<"mp4" | "mov" | "flv" | "mkv" | "ts" | "webm" | "hls">; "protocols": Array<"file" | "hls">; "video": Array<VideoCapability>; "audio"?: (Array<AudioCapability>) | null };
export type CreateSession = { "recording_id": string; "source_version": string; "capabilities": Capabilities };
export type Recording = { "recording_id": string; "source_version": string; "path": string; "room": string; "size_bytes": number; "size_text": string };
export type RecordingList = { "items": Array<Recording>; "next_cursor": string; "catalog_status": "verified" };
export type Session = { "id": string; "recording_id": string; "source_version": string; "asset_version": string; "status": "processing" | "paused" | "ready" | "failed" | "cancelled"; "decision": "direct" | "remux" | "audio-transcode" | "video-transcode"; "protocol"?: "file" | "hls"; "url"?: string; "expires_at": number; "token_expires_at"?: number; "refresh_after_seconds"?: number; "retry_after_seconds": number; "error": (Error) | null };
export type DeleteItem = { "recording_id": string; "source_version": string };
export type BatchDelete = { "items": Array<DeleteItem> };
export type DeleteResult = { "recording_id": string; "status": "deleted" | "failed"; "deleted_files": number; "error": (Error) | null };
export type BatchResult = { "items": Array<DeleteResult> };
export type RecordingsEnvelope = { "data": (RecordingList) | null; "error": (Error) | null; "request_id": string };
export type SessionEnvelope = { "data": (Session) | null; "error": (Error) | null; "request_id": string };
export type BatchEnvelope = { "data": (BatchResult) | null; "error": (Error) | null; "request_id": string };
export type ErrorEnvelope = { "data": (Record<string, unknown>) | null; "error": Error; "request_id": string };

const schemas: Record<string, any> = {"Error":{"type":"object","properties":{"code":{"type":"string"},"message":{"type":"string"},"retryable":{"type":"boolean"},"retry_after_seconds":{"type":"integer","minimum":1,"description":"可重试时提供，与 Retry-After 及会话轮询一致"}},"required":["code","message","retryable"],"additionalProperties":false},"VideoCapability":{"type":"object","properties":{"codec":{"type":"string"},"profiles":{"type":"array","items":{"type":"string","minLength":1,"maxLength":64},"minItems":1,"maxItems":16},"max_level":{"type":"integer","minimum":1,"maximum":1000},"max_width":{"type":"integer","minimum":1,"maximum":16384},"max_height":{"type":"integer","minimum":1,"maximum":16384},"max_fps":{"type":"number","minimum":0,"exclusiveMinimum":true,"maximum":240},"max_bit_depth":{"type":"integer","minimum":8,"maximum":16},"hdr":{"type":"boolean","default":false}},"required":["codec","profiles","max_level","max_width","max_height","max_fps","max_bit_depth"],"additionalProperties":false},"AudioCapability":{"type":"object","properties":{"codec":{"type":"string"},"profiles":{"type":"array","items":{"type":"string","minLength":1,"maxLength":64},"minItems":1,"maxItems":16},"max_channels":{"type":"integer","minimum":1,"maximum":32},"max_sample_rate":{"type":"integer","minimum":1,"maximum":384000}},"required":["codec","profiles","max_channels","max_sample_rate"],"additionalProperties":false},"Capabilities":{"type":"object","properties":{"containers":{"type":"array","items":{"type":"string","enum":["mp4","mov","flv","mkv","ts","webm","hls"]},"minItems":1,"maxItems":16},"protocols":{"type":"array","items":{"type":"string","enum":["file","hls"]},"minItems":1,"maxItems":16},"video":{"type":"array","items":{"$ref":"#/components/schemas/VideoCapability"},"minItems":1,"maxItems":16},"audio":{"type":"array","items":{"$ref":"#/components/schemas/AudioCapability"},"maxItems":16,"nullable":true}},"required":["containers","protocols","video"],"additionalProperties":false},"CreateSession":{"type":"object","properties":{"recording_id":{"type":"string","format":"uuid"},"source_version":{"type":"string","pattern":"^[a-f0-9]{64}$"},"capabilities":{"$ref":"#/components/schemas/Capabilities"}},"required":["recording_id","source_version","capabilities"],"additionalProperties":false},"Recording":{"type":"object","properties":{"recording_id":{"type":"string","format":"uuid"},"source_version":{"type":"string","pattern":"^[a-f0-9]{64}$"},"path":{"type":"string"},"room":{"type":"string"},"size_bytes":{"type":"integer","minimum":0},"size_text":{"type":"string"}},"required":["recording_id","source_version","path","room","size_bytes","size_text"],"additionalProperties":false},"RecordingList":{"type":"object","properties":{"items":{"type":"array","items":{"$ref":"#/components/schemas/Recording"}},"next_cursor":{"type":"string"},"catalog_status":{"type":"string","enum":["verified"]}},"required":["items","next_cursor","catalog_status"],"additionalProperties":false},"Session":{"type":"object","properties":{"id":{"type":"string","format":"uuid"},"recording_id":{"type":"string","format":"uuid"},"source_version":{"type":"string","pattern":"^[a-f0-9]{64}$"},"asset_version":{"type":"string"},"status":{"type":"string","enum":["processing","paused","ready","failed","cancelled"]},"decision":{"type":"string","enum":["direct","remux","audio-transcode","video-transcode"]},"protocol":{"type":"string","enum":["file","hls"]},"url":{"type":"string"},"expires_at":{"type":"integer"},"token_expires_at":{"type":"integer"},"refresh_after_seconds":{"type":"integer","minimum":0},"retry_after_seconds":{"type":"integer","minimum":0},"error":{"allOf":[{"$ref":"#/components/schemas/Error"}],"nullable":true,"type":"object"}},"required":["id","recording_id","source_version","asset_version","status","decision","expires_at","retry_after_seconds","error"],"additionalProperties":false},"DeleteItem":{"type":"object","properties":{"recording_id":{"type":"string","format":"uuid"},"source_version":{"type":"string","pattern":"^[a-f0-9]{64}$"}},"required":["recording_id","source_version"],"additionalProperties":false},"BatchDelete":{"type":"object","properties":{"items":{"type":"array","items":{"$ref":"#/components/schemas/DeleteItem"},"minItems":1,"maxItems":100}},"required":["items"],"additionalProperties":false},"DeleteResult":{"type":"object","properties":{"recording_id":{"type":"string","format":"uuid"},"status":{"type":"string","enum":["deleted","failed"]},"deleted_files":{"type":"integer","minimum":0},"error":{"allOf":[{"$ref":"#/components/schemas/Error"}],"nullable":true,"type":"object"}},"required":["recording_id","status","deleted_files","error"],"additionalProperties":false},"BatchResult":{"type":"object","properties":{"items":{"type":"array","items":{"$ref":"#/components/schemas/DeleteResult"}}},"required":["items"],"additionalProperties":false},"RecordingsEnvelope":{"type":"object","properties":{"data":{"allOf":[{"$ref":"#/components/schemas/RecordingList"}],"nullable":true,"type":"object"},"error":{"allOf":[{"$ref":"#/components/schemas/Error"}],"nullable":true,"type":"object"},"request_id":{"type":"string","minLength":1}},"required":["data","error","request_id"],"additionalProperties":false},"SessionEnvelope":{"type":"object","properties":{"data":{"allOf":[{"$ref":"#/components/schemas/Session"}],"nullable":true,"type":"object"},"error":{"allOf":[{"$ref":"#/components/schemas/Error"}],"nullable":true,"type":"object"},"request_id":{"type":"string","minLength":1}},"required":["data","error","request_id"],"additionalProperties":false},"BatchEnvelope":{"type":"object","properties":{"data":{"allOf":[{"$ref":"#/components/schemas/BatchResult"}],"nullable":true,"type":"object"},"error":{"allOf":[{"$ref":"#/components/schemas/Error"}],"nullable":true,"type":"object"},"request_id":{"type":"string","minLength":1}},"required":["data","error","request_id"],"additionalProperties":false},"ErrorEnvelope":{"type":"object","properties":{"data":{"type":"object","nullable":true},"error":{"$ref":"#/components/schemas/Error"},"request_id":{"type":"string","minLength":1}},"required":["data","error","request_id"],"additionalProperties":false}};

// 运行时也核验契约，防止未知状态被 TypeScript 断言掩盖。
function validate(value: any, schema: any, path = 'response'): void {
  if (value === null && schema.nullable) return;
  if (schema.$ref) return validate(value, schemas[schema.$ref.split('/').pop()], path);
  if (schema.allOf) { schema.allOf.forEach((s: any) => validate(value, s, path)); return; }
  const invalid = () => { throw new Error(`v2 契约无效: ${path}`); };
  if (schema.enum && !schema.enum.includes(value)) invalid();
  if (schema.type === 'object') {
    if (!value || typeof value !== 'object' || Array.isArray(value)) invalid();
    (schema.required || []).forEach((key: string) => { if (!(key in value)) invalid(); });
    Object.entries(value).forEach(([key, v]) => {
      if (schema.properties?.[key]) validate(v, schema.properties[key], `${path}.${key}`);
      else if (schema.additionalProperties === false) invalid();
    });
  } else if (schema.type === 'array') {
    if (!Array.isArray(value) || (schema.minItems !== undefined && value.length < schema.minItems) || (schema.maxItems !== undefined && value.length > schema.maxItems)) invalid();
    value.forEach((v: any, i: number) => validate(v, schema.items, `${path}[${i}]`));
  } else if (schema.type === 'integer' || schema.type === 'number') {
    if (typeof value !== 'number' || !Number.isFinite(value) || (schema.type === 'integer' && !Number.isInteger(value)) || (schema.minimum !== undefined && value < schema.minimum) || (schema.maximum !== undefined && value > schema.maximum)) invalid();
  } else if (schema.type === 'string') {
    if (typeof value !== 'string' || (schema.minLength !== undefined && value.length < schema.minLength) || (schema.pattern && !new RegExp(schema.pattern).test(value))) invalid();
  } else if (schema.type === 'boolean' && typeof value !== 'boolean') invalid();
}
export class V2HTTPError extends Error {
  constructor(public status: number, public failure: Error, public requestId: string) { super(failure.message); }
  get code(): string { return this.failure.code; }
}
export class MissingV2Endpoint extends Error {}
export class V2Client {
  constructor(private transport: typeof fetch = authFetch) {}
  getV2Recordings(options: { query?: { after?: string; limit?: number } } = {}): Promise<RecordingsEnvelope> { return this.request("GET", "/api/v2/recordings", "RecordingsEnvelope", options); }
  createV2PlaybackSession(options: { body: CreateSession }): Promise<SessionEnvelope> { return this.request("POST", "/api/v2/playback-sessions", "SessionEnvelope", options); }
  getV2PlaybackSession(options: { params: { id: string } }): Promise<SessionEnvelope> { return this.request("GET", "/api/v2/playback-sessions/{id}", "SessionEnvelope", options); }
  cancelV2PlaybackSession(options: { params: { id: string } }): Promise<SessionEnvelope> { return this.request("DELETE", "/api/v2/playback-sessions/{id}", "SessionEnvelope", options); }
  batchDeleteV2Recordings(options: { body: BatchDelete }): Promise<BatchEnvelope> { return this.request("POST", "/api/v2/recordings/batch-delete", "BatchEnvelope", options); }
  private async request(method: string, template: string, schema: string, options: { params?: Record<string, string>; query?: Record<string, string | number>; body?: unknown }): Promise<any> {
    let url = template.replace(/\{([^}]+)\}/g, (_, key: string) => {
      if (!options.params?.[key]) throw new globalThis.Error(`缺少路径参数: ${key}`);
      return encodeURIComponent(options.params[key]);
    });
    const query = new URLSearchParams();
    Object.entries(options.query || {}).forEach(([key, value]) => { if (value !== undefined) query.set(key, String(value)); });
    if (query.toString()) url += `?${query}`;
    const response = await this.transport(url, { method, redirect: 'error', headers: options.body ? { 'Content-Type': 'application/json' } : undefined, body: options.body ? JSON.stringify(options.body) : undefined });
    const text = await response.text();
    if ((response.status === 404 && text.trim() === '404 page not found') || response.status === 405) throw new MissingV2Endpoint('服务端未提供 v2');
    let envelope: any;
    try { envelope = JSON.parse(text); } catch { throw new globalThis.Error(`v2 契约无效: HTTP ${response.status}`); }
    validate(envelope, schemas[response.ok ? schema : 'ErrorEnvelope']);
    if (!response.ok || envelope.error) {
      if (!envelope.error) throw new globalThis.Error('v2 契约无效: 缺少错误');
      throw new V2HTTPError(response.status, envelope.error, envelope.request_id);
    }
    if (envelope.data === null) throw new globalThis.Error('v2 契约无效: 缺少 data');
    return envelope;
  }
}
