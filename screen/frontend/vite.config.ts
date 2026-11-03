import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { apiProxy } from '../../web/vite.shared.ts';

// Wails serves the build from its own scheme, so assets use relative paths.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  server: { port: 5175, strictPort: true, proxy: apiProxy() },
});
