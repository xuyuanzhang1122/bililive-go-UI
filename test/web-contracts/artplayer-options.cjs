const assert=require('assert/strict'),fs=require('fs');
const Artplayer=require('../../src/webapp/node_modules/artplayer').default;
const source=fs.readFileSync('src/webapp/src/component/video-library/index.tsx','utf8');
const expression=source.match(/type:\s*(resolvedPlay\?\.protocol[^\n]+),/)[1];
const typeFor=Function('resolvedPlay',`return (${expression});`);
for(const protocol of ['file','hls',undefined]) {
 const type=typeFor({protocol});
 assert.doesNotThrow(()=>Artplayer.validator({type},{type:Artplayer.scheme.type}),`actual ArtPlayer option validator: ${protocol}`);
 if(protocol==='hls')assert.equal(type,'m3u8');
}
console.log('PASS: actual ArtPlayer file/HLS/legacy option type validation');
