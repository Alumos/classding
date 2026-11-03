import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { apiProxy } from '../../vite.shared.ts';

export default defineConfig({
  base: '/admin/',
  plugins: [react(), tailwindcss()],
  server: { port: 5174, proxy: apiProxy() },
});
