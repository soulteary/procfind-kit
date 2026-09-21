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
// Only systems with a procfs (Linux) are supported. Everywhere else every
// lookup reports "not found" rather than guessing.
package procfind

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Spec says how to recognize a process that belongs to a directory. Paths are
// relative to the directory being searched.
//
// Both lists matter, and the split is not cosmetic -- see Find for why the
// order of the result depends on it.
type Spec struct {
	// Scripts are shell scripts, such as "run.sh" or "bin/supervise.sh".
	//
	// A script started through its shebang shows up in the process table as
	// the interpreter running the script: argv is ["/bin/bash" "/srv/app/run.sh"],
	// so the script path is argv[1] and argv[0] is a shell. A script exec'd
	// directly shows up with the script itself as argv[0]. Both are matched.
	Scripts []string

	// Executables are binaries started by absolute path, such as
	// "bin/Runner.Listener". Their argv[0] is the path itself.
	Executables []string
}

// Scanner reads a procfs mount. The zero value reads /proc, which is what
// nearly every caller wants; Root exists so tests can point at a fixture
// directory without mounting anything.
type Scanner struct {
	// Root is the procfs mount point. Empty means "/proc".
	Root string
}

func (s Scanner) root() string {
	if s.Root == "" {
		return "/proc"
	}
	return s.Root
}

// Find returns the pids of processes belonging to dir, scripts first and
// executables after.
//
// That order is the point of separating the two lists. A supervisor script
// typically restarts its worker in a loop, so a caller shutting the whole
// thing down has to signal the script first; signal the worker first and the
// loop cheerfully starts another one. Callers that just want "is anything
// alive" can ignore the order, or use Running.
//
// A nil or empty result means nothing was found -- including on systems with
// no procfs, where nothing can be found by this method at all.
func (s Scanner) Find(dir string, spec Spec) []int {
	return s.FindMany([]string{dir}, spec)[dir]
}

// FindMany is Find for several directories at once, reading the process table
// only once no matter how many directories are asked about.
//
// The one-scan property is why this is the primitive and Find is the wrapper:
// a caller listing the state of N directories would otherwise read every
// entry in /proc N times, which on a busy host is the most expensive thing it
// does.
//
// Results are keyed by the exact strings passed in. Two spellings of the same
// directory each get their own entry, holding the same pids.
func (s Scanner) FindMany(dirs []string, spec Spec) map[string][]int {
	targets := resolveTargets(dirs, spec)
	if len(targets) == 0 {
		return nil
	}

	entries, err := os.ReadDir(s.root())
	if err != nil {
		// No procfs: report "not found" rather than guess. On Windows, and
		// anywhere procfs is not mounted, this is the honest answer.
		return nil
	}

	scripts := make(map[string][]int, len(targets))
	executables := make(map[string][]int, len(targets))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		argv := readArgv(s.root(), pid)
		if len(argv) == 0 {
			continue
		}
		for _, tg := range targets {
			k := classify(argv, tg.scripts, tg.executables)
			if k == kindNone {
				continue
			}
			for _, key := range tg.keys {
				if k == kindExecutable {
					executables[key] = append(executables[key], pid)
				} else {
					scripts[key] = append(scripts[key], pid)
				}
			}
			// Targets are deduplicated by absolute path, so a process can
			// only belong to one of them. Once claimed, stop comparing.
			break
		}
	}

	out := make(map[string][]int, len(targets))
	for _, tg := range targets {
		for _, key := range tg.keys {
			sup, exe := scripts[key], executables[key]
			if len(sup) == 0 && len(exe) == 0 {
				continue
			}
			sort.Ints(sup)
			sort.Ints(exe)
			out[key] = append(sup, exe...)
		}
	}
	return out
}

// Running reports whether any process belonging to dir is alive.
func (s Scanner) Running(dir string, spec Spec) bool {
	return len(s.Find(dir, spec)) > 0
}

// Find is Scanner{}.Find.
func Find(dir string, spec Spec) []int { return Scanner{}.Find(dir, spec) }

// FindMany is Scanner{}.FindMany.
func FindMany(dirs []string, spec Spec) map[string][]int { return Scanner{}.FindMany(dirs, spec) }

// Running is Scanner{}.Running.
func Running(dir string, spec Spec) bool { return Scanner{}.Running(dir, spec) }

// target is one directory, with its Spec resolved to absolute paths.
type target struct {
	keys        []string // every caller-supplied spelling of this directory
	scripts     []string
	executables []string
}

func resolveTargets(dirs []string, spec Spec) []target {
	targets := make([]target, 0, len(dirs))
	byAbs := make(map[string]int, len(dirs))

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		if i, ok := byAbs[abs]; ok {
			targets[i].keys = append(targets[i].keys, dir)
			continue
		}
		byAbs[abs] = len(targets)
		targets = append(targets, target{
			keys:        []string{dir},
			scripts:     joinAll(abs, spec.Scripts),
			executables: joinAll(abs, spec.Executables),
		})
	}
	return targets
}

func joinAll(dir string, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

type kind int

const (
	kindNone kind = iota
	kindScript
	kindExecutable
)

// classify decides whether a process belongs to a target, given its argv.
//
// Only argv[0] and argv[1] are considered, and argv[1] counts only when
// argv[0] is a shell. Scanning the whole command line instead would match
// `grep /srv/app/run.sh` and `tail -f /srv/app/run.sh` -- commands that merely
// mention the path, and that an operator is quite likely to be running at
// exactly the moment they are debugging why the supervisor keeps restarting
// things.
func classify(argv, scripts, executables []string) kind {
	if len(argv) == 0 {
		return kindNone
	}
	for _, exe := range executables {
		if argv[0] == exe {
			return kindExecutable
		}
	}
	for _, script := range scripts {
		// exec'd directly, without going through the shebang
		if argv[0] == script {
			return kindScript
		}
		if len(argv) >= 2 && argv[1] == script && isShell(argv[0]) {
			return kindScript
		}
	}
	return kindNone
}

// shells are the interpreter names accepted as argv[0] for a script.
// Compared by base name only, because bash lives in /bin on some
// distributions and /usr/bin on others.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true,
	"ksh": true, "ash": true, "busybox": true,
}

func isShell(path string) bool { return shells[filepath.Base(path)] }

// readArgv reads /proc/<pid>/cmdline and splits it on NUL.
//
// cmdline rather than readlink of /proc/<pid>/exe: cmdline is readable by any
// user on the host, while reading the exe link for a process owned by someone
// else fails with EACCES unless the caller holds CAP_SYS_PTRACE. A supervisor
// running as an ordinary service account should not need that capability to
// answer "is my worker alive".
func readArgv(root string, pid int) []string {
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil
	}
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		// Kernel threads have an empty cmdline.
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	argv := make([]string, 0, len(parts))
	for _, p := range parts {
		argv = append(argv, string(p))
	}
	return argv
}
