# Security Policy

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it through GitHub's private vulnerability reporting: go to the
[Security tab](https://github.com/soulteary/procfind-kit/security) and choose
**Report a vulnerability**. That opens a private advisory visible only to the
maintainers.

If you do not see that option, open a normal issue saying only that you have a
security report and need a private channel — **no details, no reproducer** —
and a maintainer will arrange one.

Please include, once you have a private channel:

- the affected version or commit,
- what an attacker can do, and what they need in order to do it,
- a reproducer, if you have one.

Expect an acknowledgement within a few days. This is a small
volunteer-maintained project, so please allow reasonable time for a fix before
disclosing publicly.

## Supported versions

| Version | Supported |
| ------- | --------- |
| 1.x     | ✅ |

Fixes land on the latest minor of the current major. There are no long-term
support branches. Until `v1.0.0` is tagged, `main` is that version.

## What this library does, and what it does not

procfind reads the process table and returns pids. Both halves of that have
consequences.

### argv is attacker-controlled, so a match is not proof of identity

A process's command line is set by whoever started it, and any local user can
set it to anything — `exec -a /srv/runners/one/bin/Runner.Listener sleep 3600`
is a one-liner. The path does not have to exist, be executable, or be owned by
anyone in particular: `/proc/<pid>/cmdline` is a string, and this package
compares strings.

So an unprivileged local user can make `Running(dir, spec)` answer **true** for
a directory whose real process is dead. A supervisor whose logic is "already
running, do not start" can be kept from starting that way. They cannot make it
answer false, and they cannot impersonate a process to anything that checks
ownership — but if "is it alive" gates something that matters, confirm with
something the user cannot forge: the pid's owner (`/proc/<pid>/status`), a
socket it holds, or an answer from the service itself.

This is a deliberate trade. The package exists for programs that write no pid
file, where the alternative is not a stronger check but no check at all.

### Pids are returned, not held — mind the reuse window

`Find` returns pids that a caller will typically signal. Nothing here holds a
handle on those processes, so between the scan and the signal a pid can be
recycled by the kernel and belong to something else entirely.

The window is small and it is real, and it widens with anything slow in
between — a confirmation prompt, a queue, a retry loop. Re-check immediately
before signalling, and prefer signalling the process group of a script you
started over signalling pids you discovered.

Make the `Spec` as specific as you can for the same reason: every path in it is
a way for something that is not yours to be matched, and the caller's next move
is usually `kill`.

### It reads other processes' command lines, and never reports them

On a default Linux, `/proc/<pid>/cmdline` is world-readable, so this package
reads the command lines of processes belonging to other users. Command lines
routinely contain things that should not be copied anywhere — tokens and
passwords passed as arguments.

**The package returns pids only.** No argv it reads is returned, logged, or
included in an error, and that is worth preserving in any change to it.

`cmdline` is also read *instead of* following `/proc/<pid>/exe`, which would be
the more reliable identification: reading that link for a process owned by
someone else fails with `EACCES` unless the caller holds `CAP_SYS_PTRACE`. A
supervisor running as an ordinary service account should not need that
capability to answer "is my worker alive", so the package accepts the weaker
signal and stays unprivileged.

If the host mounts `/proc` with `hidepid=1` or `hidepid=2`, processes owned by
other users are invisible and lookups for them report "not found" — the same
answer as on a system with no procfs at all.

### Matching is by path string, not by identity

`Spec` paths are joined onto the directory and compared literally. A symlink, a
bind mount, or a different spelling of the same file does not match. That is
the safe direction for a caller that signals what it finds — prefer missing a
process over killing the wrong one — but it means `Running` can answer false
for a process that really is yours, reached by another name.
