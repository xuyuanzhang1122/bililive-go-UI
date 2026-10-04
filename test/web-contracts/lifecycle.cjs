require('./setup.cjs');const assert=require('assert/strict'),fs=require('fs');
const ready=JSON.parse(fs.readFileSync('docs/contracts/examples/v2-session-ready.json')).value.data;
(async()=>{
 const {WebPlaybackSession,refreshVideo}=require('../../src/webapp/src/utils/playback-session.ts');
 let create=0,get=0,cancel=[],applied=[],errors=[],ticks=[];
 const api={mode:'v2',assertCurrent(){},client:{createV2PlaybackSession:async()=>({data:{...ready,id:String(++create)}}),getV2PlaybackSession:async({params})=>({data:{...ready,id:params.id}}),cancelV2PlaybackSession:async({params})=>{cancel.push(params.id)}}};
 const sleep=(seconds,signal)=>new Promise(resolve=>{ticks.push({seconds,resolve})});
 const model=new WebPlaybackSession(api,async v=>{applied.push(v)},e=>errors.push(e),sleep);
 await model.start({rel_path:'平台/主播/clip.mp4',recording_id:ready.recording_id,source_version:ready.source_version},{video:[{}],containers:['mp4'],protocols:['file'],audio:[]});
 await new Promise(r=>setTimeout(r,0));assert.equal(applied.length,1);assert.equal(ticks[0].seconds,240);
 ticks.shift().resolve();await new Promise(r=>setTimeout(r,0));assert.equal(applied.length,2);
 await model.close();ticks.shift().resolve();await new Promise(r=>setTimeout(r,0));assert.equal(applied.length,2);assert.deepEqual(cancel,['1']);assert.equal(errors.length,0);
 const {V2HTTPError}=require('../../src/webapp/src/utils/generated-api-v2.ts');
 api.client.getV2PlaybackSession=async()=>{get++;throw new V2HTTPError(410,{code:'session_expired',message:'expired',retryable:false},'r')};
 const renewed=new WebPlaybackSession(api,async v=>applied.push(v),e=>errors.push(e),sleep);
 await renewed.start({rel_path:'x',recording_id:ready.recording_id,source_version:ready.source_version},{video:[{}],containers:['mp4'],protocols:['file']});await new Promise(r=>setTimeout(r,0));ticks.shift().resolve();await new Promise(r=>setTimeout(r,0));assert.equal(create,3);assert.equal(get,1);await renewed.close();ticks.shift().resolve();
 class FakeVideo extends EventTarget {position=209;playbackRate=1.5;paused=true;src='';trace=[];pause(){this.trace.push('pause');this.paused=true}load(){this.trace.push('load');queueMicrotask(()=>this.dispatchEvent(new Event('loadedmetadata')))} async play(){this.trace.push('play');this.paused=false} set currentTime(v){this.position=v;queueMicrotask(()=>this.dispatchEvent(new Event('seeked')))}get currentTime(){return this.position}}
 const video=new FakeVideo();video.currentTime=209;await new Promise(r=>setTimeout(r,0));await refreshVideo(video,'https://local.test/new',new AbortController().signal);assert.equal(video.currentTime,209);assert.equal(video.playbackRate,1.5);assert(video.paused);assert(!video.trace.includes('play'));
 video.paused=false;await refreshVideo(video,'https://local.test/new2',new AbortController().signal);assert.deepEqual(video.trace.slice(-3),['pause','load','play']);
 console.log('PASS: Web 会话刷新、410 重建、退出取消与暂停/倍速/位置恢复');
})().catch(e=>{console.error(e);process.exitCode=1});
