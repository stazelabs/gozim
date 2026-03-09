# Contributing to gozim

## Release process

Releases are automated via [GoReleaser](https://goreleaser.com) and GitHub Actions.

**To cut a release:**

1. Ensure `main` is clean and all tests pass (`make test`).
2. Push a version tag:
   ```
   git tag v1.2.3
   git push origin v1.2.3
   ```
3. The [release workflow](.github/workflows/release.yml) triggers automatically, builds
   binaries for all platforms, and publishes a GitHub Release with archives and
   `checksums.txt`.

**Prerelease tags** (e.g. `v1.2.3-rc.1`, `v1.2.3-beta`) are detected automatically
and marked as pre-releases on GitHub.

## Local snapshot builds

To build all release artifacts locally without pushing a tag:

```
make snapshot
```

Output is written to `dist/`. Snapshot builds are not published anywhere.

## Commit message conventions

The changelog groups commits by prefix:

| Prefix  | Section     |
|---------|-------------|
| `feat`  | Features    |
| `fix`   | Bug Fixes   |
| other   | Other       |
