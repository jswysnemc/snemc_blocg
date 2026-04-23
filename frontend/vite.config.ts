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
  optimizeDeps: {
    include: [
      "prismjs",
      "prismjs/components/prism-bash",
      "prismjs/components/prism-c",
      "prismjs/components/prism-clike",
      "prismjs/components/prism-cpp",
      "prismjs/components/prism-css",
      "prismjs/components/prism-java",
      "prismjs/components/prism-javascript",
      "prismjs/components/prism-json",
      "prismjs/components/prism-lua",
      "prismjs/components/prism-jsx",
      "prismjs/components/prism-markdown",
      "prismjs/components/prism-markup",
      "prismjs/components/prism-python",
      "prismjs/components/prism-rust",
      "prismjs/components/prism-sql",
      "prismjs/components/prism-tsx",
      "prismjs/components/prism-typescript",
      "prismjs/components/prism-yaml",
      "prismjs/components/prism-go",
    ],
  },
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
