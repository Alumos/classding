import {readFileSync,writeFileSync} from 'node:fs';
import openapiTS,{astToString} from 'openapi-typescript';
const schema=JSON.parse(readFileSync(new URL('../../api/ws/frame.schema.json',import.meta.url),'utf8'));
const document={openapi:'3.1.0',info:{title:'ClassDing WebSocket',version:'0.1.1'},paths:{},components:{schemas:{Frame:schema}}};
const ast=await openapiTS(document);
writeFileSync(new URL('../../web/packages/api/src/ws-types.ts',import.meta.url),'// Generated from api/ws/frame.schema.json. Do not edit.\n'+astToString(ast));
