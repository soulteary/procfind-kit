# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Nothing is tagged yet, so `go get` resolves to a pseudo-version of `main` and
the API may still change.

## [Unreleased]

### Added

- `CHANGELOG.md` and `SECURITY.md`. The security policy states the limit of
  what a match proves — argv is set by whoever started the process, so any
  local user can make `Running` answer true — along with the pid-reuse window
  a caller signals into, and why the package reads `cmdline` rather than
  following `/proc/<pid>/exe`.
- Runnable examples (`Example`, `ExampleScanner_Find_scriptForms`,
  `ExampleScanner_Find_mentionsAreNotMatches`, `ExampleScanner_FindMany`,
  `ExampleScanner_Find_noProcfs`) that `go test` verifies, so they cannot
  drift from the API. They are an external test package
  (`package procfind_test`) and build a fake procfs under `Scanner.Root`, so
  they show exactly what `Find` returns rather than depending on whatever
  happens to be running.
- A Go Report Card workflow, run on demand, that regenerates
  `.github/goreportcard.svg` and `.github/goreportcard-report.md` and commits
  them back.
- A `.gitignore` covering build output, coverage artifacts and editor files.

### Changed

- The package doc moved from `procfind.go` to `doc.go`, and gained a layout
  section covering `Spec`, `Scanner` and why `FindMany` is the primitive, plus
  the two deliberate limits — only argv[0] and argv[1] are considered, and the
  process table is read through `cmdline` rather than the `exe` link.
- CI pins `actions/checkout`, `actions/setup-go` and `actions/upload-artifact`
  to v7, and uploads the HTML coverage report as a build artifact. There is no
  coverage service: the profile is produced and summarised inside the job, and
  the browsable report is downloadable from the run.

## Before the first release

procfind was extracted from a GitHub Actions runner supervisor. None of the
runner's three launch scripts records a pid anywhere — the value lives only in
a shell variable, and the file that looks like it might, `.path`, holds a
`PATH` string. Code that went looking for a pid file found nothing every time,
concluded the process was dead, and started a second copy.
