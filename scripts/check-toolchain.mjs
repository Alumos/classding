import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';

const root=new URL('../',import.meta.url);
const readJSON=path=>JSON.parse(readFileSync(new URL(path,root),'utf8'));
const expected=readJSON('package.json').devDependencies.typescript;
const installed=readJSON('node_modules/typescript/package.json').version;
assert.equal(installed,expected,'实际 TypeScript 版本必须与声明一致，不能被全局覆盖降级');
const generatorRequire=createRequire(new URL('scripts/api-generator/package.json',root));
const generatorTS=generatorRequire('typescript');
assert.equal(typeof generatorTS.factory?.createIdentifier,'function','接口生成器需要兼容的 TypeScript AST API');
assert.equal(typeof generatorTS.createPrinter,'function','接口生成器需要兼容的 TypeScript 打印器');
console.log(`应用 TypeScript ${installed}；接口生成器 API ${generatorTS.version}`);
