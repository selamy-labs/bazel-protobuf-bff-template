// Bundles the page TypeScript has already compiled. Only the test-identity mode can reach the
// development identity code. The entry is a script, not index.html: Bazel links inputs into its
// sandbox, and the bundler's HTML step cannot name files reached through those links.
export default ({ mode }) => ({
  build: {
    emptyOutDir: true,
    outDir: mode === "test-identity" ? "dist_dev" : "dist",
    rollupOptions: { input: "main.js", output: { entryFileNames: "main.js" } },
  },
  define: { __TEST_IDENTITY__: JSON.stringify(mode === "test-identity") },
  logLevel: "warn",
});
