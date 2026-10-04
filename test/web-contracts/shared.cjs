require('./setup.cjs');const fs=require('fs'),assert=require('assert/strict');
const {legacyResolve}=require('../../src/webapp/src/utils/playback-v2.ts');
for(const sample of JSON.parse(fs.readFileSync('docs/contracts/playback-resolve-fixtures.json'))){
 if(sample.reject) assert.throws(()=>legacyResolve(sample.response,'https://server/proxy','secret'),undefined,sample.name);
 else {const result=legacyResolve(sample.response,'https://server/proxy','secret');if(sample.url)assert.equal(result.url,sample.url,sample.name);if(sample.retry)assert.equal(result.retry_after_seconds,sample.retry,sample.name);}
}
console.log('PASS: Web 15 个共享 resolver 样例');
(async()=>{
 const {V2Client}=require('../../src/webapp/src/utils/generated-api-v2.ts');
 const client=new V2Client();let count=0;
 for(const name of fs.readdirSync('docs/contracts/examples').filter(n=>n.startsWith('v2-'))){
  const example=JSON.parse(fs.readFileSync('docs/contracts/examples/'+name));
  if(example.schema==='SessionEnvelope') {global.fetch=async()=>Response.json(example.value);const actual=await client.getV2PlaybackSession({params:{id:'x'}});assert.deepEqual(actual,example.value);count++;}
  if(example.schema==='RecordingsEnvelope') {global.fetch=async()=>Response.json(example.value);assert.deepEqual(await client.getV2Recordings(),example.value);count++;}
  if(example.schema==='ErrorEnvelope'){global.fetch=async()=>Response.json(example.value,{status:401});await assert.rejects(()=>client.getV2Recordings(),e=>e.code===example.value.error.code);count++;}
 }
 console.log(`PASS: Web ${count} 个共享 v2 响应样例`);
})().catch(e=>{console.error(e);process.exitCode=1});
