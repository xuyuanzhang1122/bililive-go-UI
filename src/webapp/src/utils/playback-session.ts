import { Capabilities, CreateSession, Session, V2HTTPError } from './generated-api-v2';
import { PlaybackAPI, RecordingFile, validateSession } from './playback-v2';

const delay = (seconds: number, signal: AbortSignal): Promise<void> => new Promise((resolve, reject) => {
  const cancel = () => { clearTimeout(timer); reject(new Error('播放已退出')); };
  const timer = setTimeout(() => { signal.removeEventListener('abort', cancel); resolve(); }, Math.min(240000, Math.max(1000, seconds * 1000)));
  if (signal.aborted) cancel(); else signal.addEventListener('abort', cancel, { once: true });
});

export class WebPlaybackSession {
  private controller = new AbortController();
  private current: Session | null = null;
  private playedAsset: string | null = null;
  constructor(private api: PlaybackAPI, private onReady: (session: Session) => Promise<void>, private onError: (error: Error) => void, private sleep = delay) {}
  private active(): boolean { return !this.controller.signal.aborted; }
  private check(value: Session, body: CreateSession): Session {
    this.api.assertCurrent();
    if (value.recording_id !== body.recording_id || value.source_version !== body.source_version) throw new Error('播放来源已变化');
    const validated = validateSession(value);
    if(validated.status==='ready' && this.playedAsset && validated.asset_version!==this.playedAsset) throw new Error('播放资产已变化，请重新打开');
    return validated;
  }
  private async create(body: CreateSession): Promise<Session> {
    const {data} = await this.api.client.createV2PlaybackSession({body});
    if (!data) throw new Error('会话缺少 data');
    if (!this.active()) { await this.cancel(data.id); throw new Error('播放已退出'); }
    this.current = data;
    return this.check(data,body);
  }
  async start(file: RecordingFile, capabilities: Capabilities): Promise<boolean> {
    try {
      if (!file.recording_id || !file.source_version) {
        const files = await this.api.list(file.rel_path.split('/').slice(0,-1).join('/'));
        if (!this.active()) return true;
        if (this.api.mode === 'legacy') return false;
        file = files.find(f=>f.rel_path===file.rel_path) || file;
      }
      if (!file.recording_id || !file.source_version) throw new Error('录播缺少稳定身份');
      if (!capabilities.video.length || !capabilities.containers.length || !capabilities.protocols.length) throw new Error('浏览器未确认原生解码能力');
      const body = {recording_id:file.recording_id,source_version:file.source_version,capabilities};
      const value = await this.create(body);
      void this.run(value,body);
    } catch (e) { if (this.active()) { await this.close();this.onError(e as Error); } }
    return true;
  }
  private async run(initial: Session, body: CreateSession): Promise<void> {
    let value=initial, failures=0;
    try {
      while (this.active()) {
        value=this.check(value,body);this.current=value;
        if (value.status==='ready') { this.playedAsset = this.playedAsset || value.asset_version;await this.onReady(value); }
        else if (value.status!=='processing'&&value.status!=='paused') throw new Error(value.error?.message || '播放会话已取消');
        await this.sleep(value.status==='ready'?Math.max(1,value.refresh_after_seconds || 1):Math.min(60,Math.max(1,value.retry_after_seconds || 2)),this.controller.signal);
        if (!this.active()) return;
        for (;;) {
          try {
            this.api.assertCurrent();
            const {data}=await this.api.client.getV2PlaybackSession({params:{id:value.id}});
            if (!this.active()) return;
            if (!data || data.id!==value.id) throw new Error('会话 ID 不匹配');
            let next=this.check(data,body);
            if (next.status==='ready' && next.refresh_after_seconds===0) {
              next=await this.create(body);await this.cancel(value.id);
            }
            if (value.asset_version && next.asset_version && value.asset_version!==next.asset_version) throw new Error('播放资产已变化，请重新打开');
            value=next;failures=0;break;
          } catch (e) {
            if (!this.active()) return;
            if (e instanceof V2HTTPError && e.status===410 && e.code==='session_expired') { value=await this.create(body);break; }
            if (e instanceof V2HTTPError && e.failure.retryable && failures++<3) { await this.sleep(e.failure.retry_after_seconds || 2,this.controller.signal);continue; }
            throw e;
          }
        }
      }
    } catch (e) { if (this.active()) { await this.close();this.onError(e as Error); } }
  }
  private async cancel(id: string): Promise<void> { try { await this.api.client.cancelV2PlaybackSession({params:{id}}); } catch { /* 取消为尽力请求，退出立即阻止本地回调。 */ } }
  async close(): Promise<void> { this.controller.abort();const value=this.current;this.current=null;if(value) await this.cancel(value.id); }
}

function waitVideo(video: HTMLVideoElement, event: string, action: () => void, signal: AbortSignal): Promise<void> {
  return new Promise((resolve,reject)=>{
    const done=()=>{ cleanup();resolve(); }, fail=()=>{cleanup();reject(new Error('刷新媒体准备失败'));};
    const cleanup=()=>{clearTimeout(timer);video.removeEventListener(event,done);video.removeEventListener('error',fail);signal.removeEventListener('abort',fail);};
    const timer=setTimeout(fail,10000);
    video.addEventListener(event,done,{once:true});video.addEventListener('error',fail,{once:true});signal.addEventListener('abort',fail,{once:true});
    if(signal.aborted)fail();else { try{action();}catch{fail();} }
  });
}
export async function refreshVideo(video: HTMLVideoElement, url: string, signal: AbortSignal): Promise<void> {
  const position=Number.isFinite(video.currentTime)?video.currentTime:0,rate=video.playbackRate,playing=!video.paused;
  video.pause();video.autoplay=false;
  await waitVideo(video,'loadedmetadata',()=>{video.src=url;video.load();},signal);
  if(position>0) await waitVideo(video,'seeked',()=>{video.currentTime=position;},signal);
  if(signal.aborted) return;
  video.playbackRate=rate;
  if(playing) await video.play();
}
