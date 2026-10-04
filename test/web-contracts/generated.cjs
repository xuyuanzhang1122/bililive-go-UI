const fs=require('fs'),assert=require('assert/strict'),ts=require('../../src/webapp/node_modules/typescript');
require.extensions['.ts']=(m,p)=>m._compile(ts.transpileModule(fs.readFileSync(p,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}).outputText,p);
global.window={location:{origin:'https://local.test'}};global.localStorage={getItem:()=> 'dummy-key'};
(async()=>{
 const {V2Client,V2HTTPError}=require('../../src/webapp/src/utils/generated-api-v2.ts');
 const sample=JSON.parse(fs.readFileSync('docs/contracts/examples/v2-session-ready.json','utf8')).value;
 let seen;
 global.fetch=async(url,init)=>{seen={url,init};return Response.json(sample)};
 const client=new V2Client();const result=await client.getV2PlaybackSession({params:{id:'a/b'}});
 assert.equal(result.data.status,'ready');assert.equal(seen.url,'/api/v2/playback-sessions/a%2Fb');
 assert.equal(new Headers(seen.init.headers).get('Authorization'),'Bearer dummy-key');
 global.fetch=async()=>Response.json({...sample,data:{...sample.data,status:'unknown'}});
 await assert.rejects(()=>client.getV2PlaybackSession({params:{id:'x'}}),/契约/);
 const error=JSON.parse(fs.readFileSync('docs/contracts/examples/v2-session-expired.json','utf8')).value;
 global.fetch=async()=>Response.json(error,{status:410});
 await assert.rejects(()=>client.getV2PlaybackSession({params:{id:'x'}}),e=>e instanceof V2HTTPError&&e.code==='session_expired'&&e.status===410&&e.requestId===error.request_id);
 console.log('PASS: OpenAPI 生成客户端、编码、Bearer、未知状态与结构化错误');
})().catch(e=>{console.error(e);process.exitCode=1});
