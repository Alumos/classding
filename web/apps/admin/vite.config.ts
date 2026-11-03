import {env} from 'node:process';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  base: '/admin/',
  plugins: [react(), tailwindcss()],
  server: {
    port: 5174,
    proxy: {
      '/api': { target: env.CLASSDING_API_URL || 'http://127.0.0.1:8080', changeOrigin: false },
      '/ws': { target: (env.CLASSDING_API_URL || 'http://127.0.0.1:8080').replace(/^http/, 'ws'), ws: true, changeOrigin: false },
    },
  },
});
