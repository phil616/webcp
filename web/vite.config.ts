import { fileURLToPath, URL } from 'node:url';
import { readFileSync } from 'node:fs';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
import AutoImport from 'unplugin-auto-import/vite';
import Components from 'unplugin-vue-components/vite';
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers';
import svgLoader from 'vite-svg-loader';
import tailwind from '@tailwindcss/vite';

// Follow 1Panel's application pipeline: compile Vue source once, share framework
// instances, and load one stylesheet before route components are displayed.
export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(
      readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim(),
    ),
  },
  plugins: [
    vue(),
    vueJsx(),
    svgLoader({ defaultImport: 'url' }),
    AutoImport({ imports: ['vue', 'vue-router'], dts: 'auto-imports.d.ts' }),
    Components({
      dirs: ['panel/src/components'],
      dts: 'src/element-components.d.ts',
      resolvers: [ElementPlusResolver({ importStyle: false })],
    }),
    tailwind(),
  ],
  resolve: {
    dedupe: ['vue', 'pinia', 'vue-router', 'vue-i18n', 'element-plus'],
    alias: {
      '@panel': fileURLToPath(new URL('./panel/src', import.meta.url)),
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  css: {
    preprocessorOptions: {
      scss: { additionalData: '@use "@/vendor/1panel/styles/var.scss" as *;\n' },
    },
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: process.env.RESTY_DEV_API || 'http://127.0.0.1:9080', changeOrigin: false },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'esnext',
    cssCodeSplit: false,
    rollupOptions: {
      output: {
        chunkFileNames: 'assets/js/[name]-[hash].js',
        entryFileNames: 'assets/js/[name]-[hash].js',
        assetFileNames: 'assets/[ext]/[name]-[hash].[ext]',
        manualChunks(id) {
          if (
            id.includes('/node_modules/element-plus/') ||
            id.includes('/node_modules/@element-plus/')
          )
            return 'vendor-element-plus';
          if (id.includes('/node_modules/codemirror/') || id.includes('/node_modules/@codemirror/'))
            return 'vendor-codemirror';
        },
      },
    },
  },
});
