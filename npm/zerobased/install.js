const fs = require("fs");
const path = require("path");

const PLATFORMS = {
  "linux-x64": "@zerobased/linux-x64",
  "linux-arm64": "@zerobased/linux-arm64",
  "darwin-arm64": "@zerobased/darwin-arm64",
  "darwin-x64": "@zerobased/darwin-x64",
};

const platform = `${process.platform === "win32" ? "win32" : process.platform === "darwin" ? "darwin" : "linux"}-${process.arch === "arm64" ? "arm64" : "x64"}`;
const pkg = PLATFORMS[platform];

if (!pkg) {
  console.error(`zerobased: unsupported platform ${platform}`);
  process.exit(1);
}

try {
  const pkgPath = path.dirname(require.resolve(`${pkg}/package.json`));
  const src = path.join(pkgPath, "zerobased");
  const dest = path.join(__dirname, "bin", "zerobased");

  fs.mkdirSync(path.join(__dirname, "bin"), { recursive: true });
  fs.copyFileSync(src, dest);
  fs.chmodSync(dest, 0o755);
} catch (e) {
  console.error(`zerobased: failed to install binary for ${platform}: ${e.message}`);
  process.exit(1);
}
