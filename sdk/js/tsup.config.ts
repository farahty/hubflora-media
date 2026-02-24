import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts", "src/react.ts"],
  format: ["esm", "cjs"],
  dts: true,
  clean: true,
  splitting: false,
  external: ["react"],
});
