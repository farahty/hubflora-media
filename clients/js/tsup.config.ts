import { defineConfig } from "tsup";

export default defineConfig([
  // Core client (no React dependency)
  {
    entry: { index: "src/index.ts" },
    format: ["esm", "cjs"],
    dts: true,
    sourcemap: true,
    clean: true,
    minify: false,
  },
  // React hooks & provider (requires React peer dep)
  {
    entry: { react: "src/react/index.ts" },
    format: ["esm", "cjs"],
    dts: true,
    sourcemap: true,
    clean: false,
    minify: false,
    external: ["react"],
    banner: {
      js: '"use client";',
    },
  },
]);
