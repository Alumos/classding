import {env} from 'node:process';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: './',
  server: {
    port: 5175, strictPort: true,
    proxy: {
      '/api': { target: (env.CLASSDING_API_URL || 'http://127.0.0.1:8080'), changeOrigin: false },
      '/ws': { target: (env.CLASSDING_API_URL || 'http://127.0.0.1:8080').replace(/^http/, 'ws'), ws: true, changeOrigin: false },
    },
  },
});
