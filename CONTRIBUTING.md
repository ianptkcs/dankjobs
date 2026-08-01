# Contributing to djobs

Thanks for your interest in contributing. Before opening a code PR, skim
`README.md` and `instructions.md` to understand the job convention (the
`~/jobs/<name>/` layout and the systemd user timer pattern) that djobs
discovers and displays.

## Reporting bugs / suggesting features

Open an [issue](../../issues/new/choose) using the appropriate template.

## Submitting a PR

1. Fork the repository.
2. Create a branch from `main`.
3. Run tests locally before opening the PR: `go test ./...`. Also run
   `go vet ./...` and make sure `go build ./...` succeeds.
4. Open the PR using the template — describe the what and why of the change.

## Versioning and changelog

djobs follows [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`).
While the project is at `0.x`, breaking changes may still land in a minor
bump; starting at `1.0.0`, only a major bump introduces breaking changes.

If your PR is a user-facing change (not docs/internal refactor), add a line
under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md). Releases are tagged as
`vX.Y.Z`, which triggers `.github/workflows/release.yml` to build binaries
and publish a GitHub Release. Tagging `vX.Y.Z-beta.N` (or `-rc.N`) instead
publishes it as a GitHub **prerelease** — visible under Releases but not
marked "Latest" — for changes you want out for testing before committing to
a stable tag.

## License

By contributing, you agree that your contribution will be licensed under
the [AGPL-3.0](LICENSE), the same license as the project.

## Code of conduct

Be respectful. Technical criticism is welcome; personal attacks are not.
