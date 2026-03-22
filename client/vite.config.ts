import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";
import runtimeErrorOverlay from "@replit/vite-plugin-runtime-error-modal";

// Vite 配置统一围绕 client/ 根目录展开，避免目录迁移后再出现一层嵌套路径。
export default defineConfig({
  plugins: [
    react(),
    runtimeErrorOverlay(),
    ...(process.env.NODE_ENV !== "production" &&
    process.env.REPL_ID !== undefined
      ? [
          await import("@replit/vite-plugin-cartographer").then((m) =>
            m.cartographer(),
          ),
          await import("@replit/vite-plugin-dev-banner").then((m) =>
            m.devBanner(),
          ),
        ]
      : []),
  ],
  resolve: {
    alias: {
      // 前端源码、共享 schema 和附带素材都固定从 client/ 相对定位。
      "@": path.resolve(import.meta.dirname, "src"),
      "@shared": path.resolve(import.meta.dirname, "shared"),
      "@assets": path.resolve(import.meta.dirname, "..", "attached_assets"),
    },
  },
  build: {
    // 构建产物统一输出到仓库根 dist/client，便于 Go 服务直接托管静态文件。
    outDir: path.resolve(import.meta.dirname, "..", "dist", "client"),
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        // 开发态默认把同源 `/api` 请求转给 Go 服务；
        // 显式设置 VITE_API_BASE_URL 时，前端会直连该地址，此代理仅作为兜底。
        target: process.env.VITE_API_BASE_URL || "http://localhost:9102",
        changeOrigin: true,
      },
    },
    fs: {
      strict: true,
      deny: ["**/.*"],
    },
  },
});
