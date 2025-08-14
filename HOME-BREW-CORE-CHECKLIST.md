# Homebrew Core submission checklist for linear-cli

Target: accept `linear-cli` into `homebrew/homebrew-core` so users can `brew install linear-cli` without adding a tap.

## Preconditions
- [ ] Project is reasonably popular or useful and actively maintained
- [ ] License is OSI-approved (MIT present in repo)
- [ ] Stable tags present (e.g., v0.1.1+, v0.2.0)
- [ ] Minimal external dependencies (builds with stock Go toolchain)
- [ ] Passing CI for all supported platforms

## Release hygiene
- [ ] Semver tags (vX.Y.Z)
- [ ] Changelog entries describing changes
- [ ] GitHub Releases with assets and notes
- [ ] Reproducible builds (static `CGO_ENABLED=0` binaries)

## Build targets
- [ ] Provide darwin (amd64, arm64) and linux (amd64, arm64) assets
- [ ] Ensure `go build` works without network access beyond module proxy (vendor if necessary, usually not for Go)

## Formula requirements
- [ ] Ruby formula named `linear-cli.rb`
- [ ] `desc`, `homepage`, `url` (source tarball of the tag), `sha256`, `license`
- [ ] `depends_on "go" => :build`
- [ ] `def install` uses `system "go", "build", "-ldflags=-s -w", "-o", bin/"linear-cli", "."`
- [ ] `test do` block that runs a simple command (e.g., `--help`) and matches output

## PR process
- [ ] Fork `homebrew/homebrew-core`
- [ ] Create a branch adding `Formula/linear-cli.rb`
- [ ] `brew audit --new-formula linear-cli` passes locally
- [ ] `brew test-bot` via CI (or rely on Homebrew CI after PR opened)
- [ ] Open PR with clear title: `linear-cli 0.1.1 (new formula)`
- [ ] Respond to review feedback (style, URLs, sha256, tests)

## Action items to prepare
- [ ] Update GitHub Actions release workflow to include darwin arm64 (Apple Silicon) and linux arm64 (if not already)
- [ ] Add a minimal `--version` flag to provide stable output for formula tests
- [ ] Add a CHANGELOG.md and include notes in GitHub Releases
- [ ] Cut a new tag (e.g., v0.1.1) for the PR

## Useful commands
- `brew style --fix Formula/linear-cli.rb`
- `brew audit --new-formula --strict linear-cli`
- `brew install --build-from-source --verbose ./Formula/linear-cli.rb`
- `brew test linear-cli --verbose`
