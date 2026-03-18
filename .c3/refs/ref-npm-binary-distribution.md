---
id: ref-npm-binary-distribution
c3-version: 4
title: npm Binary Distribution
type: ref
goal: Cross-platform Go binary distribution via npm optionalDependencies
summary: Platform-specific packages under @lagz0ne/ scope, wrapper package with postinstall
---

# npm Binary Distribution

## Goal

Cross-platform Go binary distribution via npm optionalDependencies.

## Choice

esbuild/turbo/swc pattern — platform-specific npm packages via `optionalDependencies`. npm only installs the matching platform package.

## Why

`npm install -g zerobased` is the lowest-friction install path for Node.js developers. Go cross-compilation is free. No need for separate install scripts or binary download logic.

## How

```
zerobased                          # wrapper — postinstall copies binary from platform pkg
@lagz0ne/zerobased-linux-x64      # Go binary for linux/amd64
@lagz0ne/zerobased-linux-arm64    # Go binary for linux/arm64
@lagz0ne/zerobased-darwin-arm64   # Go binary for darwin/arm64
@lagz0ne/zerobased-darwin-x64     # Go binary for darwin/amd64
```

Release workflow: tag push → cross-compile → GitHub Release + npm publish (all 5 packages).

## Scope

Applies to: `npm/` directory, `.github/workflows/release.yml`
