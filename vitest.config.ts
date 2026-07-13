import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

const repositoryRoot = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  root: repositoryRoot,
  test: {
    include: ["apps/**/*.test.ts", "sdks/**/*.test.ts", "agents/**/*.test.ts", "tests/**/*.test.ts"],
    coverage: {
      reporter: ["text", "json-summary", "html"]
    }
  }
});
