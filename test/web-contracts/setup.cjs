const fs=require('fs'),ts=require('../../src/webapp/node_modules/typescript');
require.extensions['.ts']=(m,p)=>m._compile(ts.transpileModule(fs.readFileSync(p,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}).outputText,p);
global.window={location:{origin:'https://local.test'}};global.localStorage={getItem:()=> 'dummy-key'};
