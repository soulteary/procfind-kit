// Package procfind reports which running processes belong to a given
// directory, by reading /proc and matching each process's argv against paths
// under that directory.
//
// It exists because a whole class of programs do not write a pid file. The
// GitHub Actions runner is the case this package was extracted from: none of
// its three launch scripts records a pid anywhere -- the value lives only in a
// shell variable -- and the file that looks like it might (.path) holds a PATH
// string. Code that went looking for a pid file therefore found nothing, every
// time, and concluded the process was dead. A supervisor acting on that
// conclusion starts a second copy every time it checks.
//
// The approach here is to ask the process table instead: a process belongs to
// a directory when its argv names a file inside that directory.
//
// # Layout
//
// [Spec] says how to recognize a process: [Spec.Scripts] for shell scripts,
// [Spec.Executables] for binaries started by absolute path. The split is not
// cosmetic -- [Scanner.Find] returns scripts first, and a caller shutting a
// tree down has to signal the supervising script before its worker, or the
// script's restart loop cheerfully starts another one.
//
// [Scanner] reads a procfs mount. The zero value reads /proc, which is what
// nearly every caller wants; [Scanner.Root] exists so tests can point at a
// fixture directory without mounting anything, and the package-level [Find],
// [FindMany] and [Running] are that zero value.
//
// [Scanner.FindMany] is the primitive and [Scanner.Find] the wrapper: a caller
// listing the state of N directories would otherwise read every entry in /proc
// N times, which on a busy host is the most expensive thing it does.
//
// # Getting started
//
//	spec := procfind.Spec{
//		Scripts:     []string{"run.sh", "run-helper.sh"},
//		Executables: []string{"bin/Runner.Listener"},
//	}
//
//	if pids := procfind.Find("/srv/runners/one", spec); len(pids) > 0 {
//		// pids[0] is the supervising script, if one matched.
//	}
//
//	alive := procfind.Running("/srv/runners/one", spec)
//
// # What it deliberately does not do
//
// Only argv[0] and argv[1] are considered, and argv[1] counts only when
// argv[0] is a shell. Scanning the whole command line instead would match
// `grep /srv/app/run.sh` and `tail -f /srv/app/run.sh` -- commands that merely
// mention the path, and that an operator is quite likely to be running at
// exactly the moment they are debugging why the supervisor keeps restarting
// things.
//
// The process table is read through /proc/<pid>/cmdline rather than by
// following /proc/<pid>/exe: cmdline is readable by any user on the host,
// while reading the exe link of a process owned by someone else fails with
// EACCES unless the caller holds CAP_SYS_PTRACE. A supervisor running as an
// ordinary service account should not need that capability to answer "is my
// worker alive".
//
// Only systems with a procfs (Linux) are supported. The package builds
// everywhere, and everywhere else every lookup reports "not found" rather than
// guessing.
package procfind
