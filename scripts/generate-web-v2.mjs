import fs from 'node:fs';
const spec = JSON.parse(fs.readFileSync('docs/contracts/openapi-v2.yaml', 'utf8'));
const schemas = spec.components.schemas;
function type(s) {
 let value;
 if(s.$ref) value=s.$ref.split('/').pop();
 else if(s.allOf) value=s.allOf.map(type).join(' & ');
 else if(s.enum) value=s.enum.map(v=>JSON.stringify(v)).join(' | ');
 else if(s.type==='array') value=`Array<${type(s.items)}>`;
 else if(s.type==='object') value=s.properties?`{ ${Object.entries(s.properties).map(([k,v])=>`${JSON.stringify(k)}${s.required?.includes(k)?'':'?'}: ${type(v)}`).join('; ')} }`:'Record<string, unknown>';
 else if(s.type==='integer'||s.type==='number') value='number';
 else if(s.type==='boolean') value='boolean';
 else if(s.type==='string') value='string';
 else throw new Error(`未支持的 schema: ${JSON.stringify(s)}`);
 return s.nullable?`(${value}) | null`:value;
}
const methods=[];
for(const [path,item] of Object.entries(spec.paths)) for(const [method,op] of Object.entries(item)) {
 if(!op.operationId) continue;
 const response=Object.entries(op.responses).find(([code,r])=>code.startsWith('2')&&r.content?.['application/json'])?.[1];
 if(!response) continue; // 媒体由播放器使用短期 URL 获取。
 const result=response.content['application/json'].schema.$ref.split('/').pop();
 const params=[...(item.parameters||[]),...(op.parameters||[])];
 const pathParams=params.filter(p=>p.in==='path'); const query=params.filter(p=>p.in==='query');
 const body=op.requestBody?.content['application/json'].schema;
 const options=[pathParams.length?`params: { ${pathParams.map(p=>`${p.name}: ${type(p.schema)}`).join('; ')} }`:null,query.length?`query?: { ${query.map(p=>`${p.name}${p.required?'':'?'}: ${type(p.schema)}`).join('; ')} }`:null,body?`body: ${type(body)}`:null].filter(Boolean).join('; ');
 methods.push(`  ${op.operationId}(options: { ${options} }${pathParams.length||body?'':' = {}'}): Promise<${result}> { return this.request(${JSON.stringify(method.toUpperCase())}, ${JSON.stringify(path)}, ${JSON.stringify(result)}, options); }`);
}
let code=`// 由 T5 OpenAPI 自动生成，请运行 scripts/generate-web-api.mjs。\nimport { authFetch } from './common';\n\n${Object.entries(schemas).map(([k,s])=>`export type ${k} = ${type(s)};`).join('\n')}\n\nconst schemas: Record<string, any> = ${JSON.stringify(schemas)};\n`;
code+=fs.readFileSync('scripts/web-v2-runtime.txt','utf8').replace('/* GENERATED_METHODS */',methods.join('\n'));
fs.writeFileSync('src/webapp/src/utils/generated-api-v2.ts',code);
console.log(`已从 OpenAPI 生成 ${methods.length} 个类型化 v2 管理方法`);
