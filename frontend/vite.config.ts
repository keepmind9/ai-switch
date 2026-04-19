import { resolve } from "node:path"
import vue from "@vitejs/plugin-vue"
import UnoCSS from "unocss/vite"
import AutoImport from "unplugin-auto-import/vite"
import SvgComponent from "unplugin-svg-component/vite"
import { ElementPlusResolver } from "unplugin-vue-components/resolvers"
import Components from "unplugin-vue-components/vite"
import { defineConfig, loadEnv } from "vite"
import svgLoader from "vite-svg-loader"

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "")
  return {
  base: "/ui/",
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
      "@@": resolve(__dirname, "src/common")
    }
  },
  server: {
    host: true,
    port: 3333,
    proxy: {
      "/api": {
        target: env.VITE_API_TARGET || "http://127.0.0.1:12345",
        changeOrigin: true
      }
    },
    warmup: {
      clientFiles: ["./src/layouts/**/*.*", "./src/pinia/**/*.*", "./src/router/**/*.*"]
    }
  },
  build: {
    outDir: "../internal/handler/static",
    emptyOutDir: false,
    rollupOptions: {
      output: {
        manualChunks: {
          vue: ["vue", "vue-router", "pinia"],
          element: ["element-plus", "@element-plus/icons-vue"]
        }
      }
    },
    reportCompressedSize: false,
    chunkSizeWarningLimit: 2048
  },
  esbuild: mode === "development"
    ? undefined
    : { pure: ["console.log"], drop: ["debugger"], legalComments: "none" },
  optimizeDeps: {
    include: ["element-plus/es/components/*/style/css"]
  },
  css: {
    preprocessorMaxWorkers: true
  },
  plugins: [
    vue(),
    svgLoader({
      defaultImport: "url",
      svgoConfig: {
        plugins: [{ name: "preset-default", params: { overrides: { removeViewBox: false } } }]
      }
    }),
    SvgComponent({
      iconDir: [resolve(__dirname, "src/common/assets/icons")],
      preserveColor: resolve(__dirname, "src/common/assets/icons/preserve-color"),
      dts: true,
      dtsDir: resolve(__dirname, "types/auto")
    }),
    UnoCSS(),
    AutoImport({
      imports: ["vue", "vue-router", "pinia"],
      dts: "types/auto/auto-imports.d.ts",
      resolvers: [ElementPlusResolver()]
    }),
    Components({
      dts: "types/auto/components.d.ts",
      resolvers: [ElementPlusResolver()]
    })
  ]
}})
