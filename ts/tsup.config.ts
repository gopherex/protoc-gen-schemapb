import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts", "src/schemapb/options/v1/options_pb.ts"],
  format: ["esm"],
  dts: true,
  sourcemap: true,
  clean: true,
});
