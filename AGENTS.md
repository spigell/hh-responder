# Repository Guidelines

## Project Structure & Module Organization
The CLI entrypoint lives in `main.go`, delegating to Cobra commands under `cmd/`. Use `cmd/root.go` to register top-level flags, while `cmd/run.go` orchestrates hh.ru interactions and prompts. Domain logic for vacancies, resumes, and API requests sits in `internal/headhunter/`, and cross-cutting logging helpers are in `internal/logger/`. Configuration examples such as `hh-responder-example.yaml` illustrate expected run-time inputs. Keep new packages under `internal/` when they should not be exported, or under a new top-level module only when public reuse is intended.

## Build, Test, and Development Commands
`go build -ldflags="-X 'hh-responder/cmd.version=vX.Y.Z'"` produces a versioned binary identical to the release artifacts. Use `go run . run --config ./hh-responder-example.yaml` for quick manual smoke tests once you configure the `token` value. Run `go test ./...` before every PR; add `-run` filters for focused work and `-cover` to check coverage progress. Never try to any command that can use a real HeadHunter token if you are not asked!

## Coding Style & Naming Conventions
Follow standard Go conventions: tabs for indentation, mixedCaps for exported identifiers, and short receiver names. Always run `gofmt` (or rely on IDE auto-format) before committing; prefer `goimports` to keep imports sorted. When adding configuration keys, keep them lower-case with hyphen-separated names to match existing flags (`auto-aprove`, `exclude-file`), and document environment variables in upper snake case (e.g., `HH_TOKEN`).

## Testing Guidelines
Add table-driven `_test.go` files alongside the code they exercise, placing fixtures inside the same package when possible. Focus on deterministic tests that mock external hh.ru responses; leverage the existing interfaces in `internal/headhunter` to swap in fakes. Target meaningful coverage of new logic and avoid relying on live external services in CI. Record any manual verification steps in the PR if automation is not yet feasible.

## Commit & Pull Request Guidelines
Keep commits focused and written in the imperative mood, optionally prefixed with a context tag (`ci:`, `feat:`, `fix:`) as seen in history. Reference issues in the commit body or PR description, explain user-facing changes, and include CLI examples or screenshots when they clarify behavior. Every PR should list how to test the change (commands run, configs used) and call out any follow-up work so reviewers can respond quickly.

## Develop Environment
The `deploy/develop` directory contains the Codex workbench overlay, including `overrides/codex-config.toml` for agent permissions and bootstrap scripts under `overrides/`. Update these files when adjusting local development capabilities, such as granting full modification and command execution access.

The `deploy/develop/overrides/hh-responder-dev.yaml` file is for local queries and testing only; it may hold sensitive data, so keep it out of Git, and if you need to share adjustments, create a separate sanitized config instead.
