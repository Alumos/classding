import {defineConfig} from '@playwright/test';
export default defineConfig({
  testDir:'tests/e2e',fullyParallel:false,workers:1,timeout:45000,
  expect:{timeout:12000},reporter:[['list'],['html',{open:'never'}]],
  use:{browserName:'chromium',trace:'retain-on-failure',screenshot:'only-on-failure',launchOptions:{args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream'],...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}:{})}},
  projects:[{name:'tablet',grep:/管理员登录|教师登录/,use:{viewport:{width:820,height:1180}}},{name:'desktop',use:{viewport:{width:1440,height:1000}}},{name:'mobile',use:{viewport:{width:390,height:844}}}],
  webServer:[
    {command:'node scripts/dev-server.mjs --e2e',url:'http://127.0.0.1:18080/healthz',timeout:120000,reuseExistingServer:false},
    {command:'pnpm --filter @classding/admin exec vite --host 127.0.0.1 --port 15174 --strictPort',url:'http://127.0.0.1:15174/admin/',env:{CLASSDING_API_URL:'http://127.0.0.1:18080'},reuseExistingServer:false},
    {command:'pnpm --filter @classding/teacher exec vite --host 127.0.0.1 --port 15173 --strictPort',url:'http://127.0.0.1:15173',env:{CLASSDING_API_URL:'http://127.0.0.1:18080'},reuseExistingServer:false},
    {command:'pnpm --filter @classding/screen exec vite --host 127.0.0.1 --port 15175 --strictPort',url:'http://127.0.0.1:15175',env:{CLASSDING_API_URL:'http://127.0.0.1:18080'},reuseExistingServer:false}
  ]
});
