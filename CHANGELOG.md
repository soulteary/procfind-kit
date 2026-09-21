# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] — unreleased

First tagged release. From here the exported API is a compatibility promise:
it will not change incompatibly before 2.0.0.

procfind was extracted from a GitHub Actions runner supervisor. None of the
runner's three launch scripts records a pid anywhere — the value lives only in
a shell variable, and the file that looks like it might, `.path`, holds a
`PATH` string. Code that went looking for a pid file found nothing every time,
concluded the process was dead, and started a second copy.

**Requires Go 1.27 or newer.** The kits track the current Go release together.
Note that a library's `go` directive is a hard minimum for everyone who imports
it: `go get` raises the consumer's own `go.mod` to match. The package builds on
every platform; without a procfs to read — on Windows, or anywhere `/proc` is
not mounted — every lookup reports "not found" rather than guessing.

### What 1.0.0 provides

- `Spec`, describing how to recognize a process that belongs to a directory,
  with `Scripts` and `Executables` as separate lists. The split is not
  cosmetic: `Find` returns scripts first, and a caller shutting a tree down has
  to signal the supervising script before its worker, or the script's restart
  loop starts another one.
- `Scanner`, with `Root` so a caller can point at a fixture instead of `/proc`,
  and the package-level `Find`, `FindMany` and `Running` over its zero value.
- `FindMany` as the primitive and `Find` as the wrapper: the process table is
  read once no matter how many directories are asked about.
- Matching on argv[0], and argv[1] only when argv[0] is a shell, so a command
  that merely mentions the path — `grep`, `tail -f`, an editor — is not a
  match. Shells are compared by base name, because bash lives in `/bin` on some
  distributions and `/usr/bin` on others.

### Also in the repository

- `SECURITY.md`, which states the limit of what a match proves: argv is set by
  whoever started the process, so any local user can make `Running` answer true
  for a directory whose real process is dead. It also covers the pid-reuse
  window a caller signals into, why the package reads `cmdline` rather than
  following `/proc/<pid>/exe`, and that no argv it reads is ever returned or
  logged.
- Runnable examples in `example_test.go` that `go test` verifies, as an
  external test package building a fake procfs under `Scanner.Root`, so they
  show exactly what `Find` returns and cannot drift from the exported API.
- CI covering formatting, vet, tests, golangci-lint and govulncheck, with the
  test job run on Linux and macOS, and a `GOOS=windows` build to keep the
  package compiling where there is no procfs. Every job takes its Go version
  from `go.mod`. The HTML coverage report is uploaded as a build artifact; no
  coverage service is involved.
- A Go Report Card workflow, run on demand, that regenerates the badge and
  report and commits them back.

[1.0.0]: https://github.com/soulteary/procfind-kit/releases/tag/v1.0.0
