import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
  root: 'src/pages',
  publicDir: resolve(__dirname, 'public'),
  build: {
    outDir: resolve(__dirname, 'proxy/dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: {
        dashboard: resolve(__dirname, 'src/pages/dashboard/index.html'),
        browser: resolve(__dirname, 'src/pages/browser/index.html'),
        viewer: resolve(__dirname, 'src/pages/viewer/index.html'),
        reader: resolve(__dirname, 'src/pages/reader/index.html'),
        hosters: resolve(__dirname, 'src/pages/hosters/index.html'),
        preview: resolve(__dirname, 'src/pages/preview/index.html'),
        scalar: resolve(__dirname, 'src/pages/scalar/index.html'),
      },
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/v1': 'http://localhost:8080',
      '/api': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
      '/dl': 'http://localhost:8080',
      '/icon.png': 'http://localhost:8080',
      '/favicon.ico': 'http://localhost:8080',
    },
  },
});
