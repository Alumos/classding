import {cpSync,mkdirSync,rmSync} from 'node:fs';
for(const app of ['teacher','admin']){const dest=`server/static/${app}`;rmSync(dest,{recursive:true,force:true});mkdirSync(dest,{recursive:true});cpSync(`web/apps/${app}/dist`,dest,{recursive:true});}
