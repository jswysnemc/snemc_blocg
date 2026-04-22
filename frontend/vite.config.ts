import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { vitePluginForArco } from "@arco-plugins/vite-vue";
import path from "node:path";

export default defineConfig({
  base: "/front/",
  plugins: [
    vue(),
    vitePluginForArco({ style: "css" }),
  ],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    manifest: false,
    rollupOptions: {
      input: {
        admin: path.resolve(__dirname, "admin.html"),
        public: path.resolve(__dirname, "src/public-entry.ts"),
      },
      output: {
        entryFileNames: "assets/[name].js",
        chunkFileNames: "assets/chunk-[name].js",
        assetFileNames: ({ name }) => {
          if (name?.endsWith(".css")) {
            return "assets/[name][extname]";
          }
          return "assets/[name]-[hash][extname]";
        },
      },
    },
  },
});
