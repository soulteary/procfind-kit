# procfind-kit

[![CI](https://github.com/soulteary/procfind-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/soulteary/procfind-kit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/procfind-kit.svg)](https://pkg.go.dev/github.com/soulteary/procfind-kit)

Find the running processes that belong to a directory, for programs that don't write a pid file. Zero dependencies.

**Docs:** English · [中文](README_CN.md)

## The problem

A supervisor needs to answer one question: *is my worker still alive?* The usual answer is a pid file — but a surprising number of programs don't write one.

The case this package was extracted from is the GitHub Actions runner. None of its three launch scripts records a pid anywhere; the value lives only in a shell variable. The file that looks like it might, `.path`, holds a `PATH` string. Code that went looking for a pid file therefore found nothing — **every time, for every runner** — and concluded the process was dead.

A supervisor acting on that conclusion starts a second copy. Every time it checks.

This package asks the process table instead: a process belongs to a directory when its `argv` names a file inside that directory.

## Install

```bash
go get github.com/soulteary/procfind-kit
```

## Usage

Describe how a process of yours can be recognized, then ask:

```go
import procfind "github.com/soulteary/procfind-kit"

spec := procfind.Spec{
    Scripts:     []string{"run.sh", "run-helper.sh"},
    Executables: []string{"bin/Runner.Listener"},
}

if procfind.Running("/srv/runners/build-01", spec) {
    // ...
}

pids := procfind.Find("/srv/runners/build-01", spec)
```

Listing many directories at once reads the process table **once**, not once per directory:

```go
byDir := procfind.FindMany([]string{"/srv/a", "/srv/b", "/srv/c"}, spec)
for dir, pids := range byDir {
    fmt.Println(dir, pids)
}
```

## Why `Scripts` and `Executables` are separate lists

Two reasons, both learned the hard way.

**They appear differently in the process table.** A script started through its shebang is the *interpreter* running the script, so `argv` is `["/bin/bash", "/srv/app/run.sh"]` — the path is `argv[1]`. A binary started by absolute path has the path as `argv[0]`. A script exec'd directly (no shebang) also has it as `argv[0]`. All three are matched.

**The result is ordered scripts-first.** A supervisor script usually restarts its worker in a loop. A caller shutting the whole thing down has to signal the script first; signal the worker first and the loop cheerfully starts another one. `Find` returns scripts before executables so that

```go
for _, pid := range procfind.Find(dir, spec) {
    syscall.Kill(pid, syscall.SIGTERM)
}
```

does the right thing without the caller having to know the rule.

## What it deliberately does not do

**It only looks at `argv[0]` and `argv[1]`, and only trusts `argv[1]` when `argv[0]` is a shell.** Scanning the whole command line would match `grep /srv/app/run.sh` and `tail -f /srv/app/run.sh` — commands an operator is quite likely to be running at exactly the moment they're debugging why the supervisor keeps restarting things. Counting those as "the service is up" would hide the outage.

**It reads `cmdline`, not the `exe` symlink.** `/proc/<pid>/cmdline` is readable by any user on the host; `readlink /proc/<pid>/exe` for a process owned by someone else fails with `EACCES` unless the caller holds `CAP_SYS_PTRACE`. A supervisor running as an ordinary service account should not need that capability to answer "is my worker alive".

**Linux only.** Without a procfs there is no way to answer, so every lookup reports "not found" — the honest answer, rather than a guess in either direction.

## Testing

`Scanner.Root` points the scan at a different procfs mount, so your own tests can build a fixture directory instead of starting real processes:

```go
s := procfind.Scanner{Root: "testdata/proc"}
pids := s.Find("/srv/app", spec)
```

The package-level `Find` / `FindMany` / `Running` are `Scanner{}` with `Root` defaulting to `/proc`.

## API

| Function | Description |
|---|---|
| `Find(dir string, spec Spec) []int` | Pids belonging to `dir`, scripts first |
| `FindMany(dirs []string, spec Spec) map[string][]int` | Same for several directories, one scan of the process table |
| `Running(dir string, spec Spec) bool` | Whether anything belonging to `dir` is alive |
| `Scanner{Root}` | The same three methods against a given procfs mount |
| `Spec{Scripts, Executables}` | Paths, relative to the directory, that identify a process |

`FindMany` keys its result by the exact strings passed in — two spellings of the same directory each get an entry, holding the same pids.

## License

Apache 2.0 — see [LICENSE](LICENSE).
