import {readFileSync} from 'node:fs';
import {env} from 'node:process';
import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
const {version}=JSON.parse(readFileSync(new URL('../../../package.json',import.meta.url),'utf8')) as {version:string};
export default defineConfig({define:{__APP_VERSION__:JSON.stringify(version)},plugins:[react(),tailwindcss()],server:{host:'0.0.0.0',port:5173,proxy:{'/api':{target:env.CLASSDING_API_URL || 'http://127.0.0.1:8080',changeOrigin:false},'/ws':{target:(env.CLASSDING_API_URL || 'http://127.0.0.1:8080').replace(/^http/, 'ws'),ws:true,changeOrigin:false}}},build:{outDir:'dist'}});
