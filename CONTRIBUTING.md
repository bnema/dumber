# Contributing to Dumber

Thanks for your interest in contributing to Dumber!

## What We Accept

| Type | Accepted |
|------|----------|
| Bug fixes | Yes |
| Performance optimizations | Yes |
| Stability improvements | Yes |
| WebUI/UX improvements | Yes |
| New features | No |

We're not accepting new feature contributions. Dumber follows a specific vision and feature set that we maintain internally. If you have a feature idea, feel free to open an issue for discussion, but PRs adding new features will be closed.

## Pull Request Process

**Target the `next` branch, not `main`.**

All PRs must be opened against the `next` branch. We test changes in `next` to ensure stability before merging to `main` for release.

```bash
git checkout -b my-fix
# make your changes
git push origin my-fix
# open PR against `next` branch
```

## Before Submitting

**Go code:**
1. Run `make lint` and fix any issues
2. Run `make test` and ensure tests pass
3. Run `go fmt` on any Go code changes

**WebUI code (in `webui/`):**
1. Run `npm run lint` to fix linting issues
2. Run `npm run fmt` to format with Prettier
3. Run `npm run check` to run svelte-check

Keep commits focused and use single-line commit messages.

## Reporting Bugs

Open an issue at [github.com/bnema/dumber/issues](https://github.com/bnema/dumber/issues) with:
- Steps to reproduce
- Expected vs actual behavior
- System info (distro, Wayland compositor, WebKitGTK version)
