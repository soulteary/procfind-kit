package procfind_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/soulteary/procfind-kit"
)

// runners is the directory the examples ask about. Nothing has to exist on
// disk: a process belongs to a directory when its argv names a file inside
// that directory, which is a question about the process table, not the
// filesystem.
const runners = "/srv/runners/one"

var spec = procfind.Spec{
	Scripts:     []string{"run.sh", "run-helper.sh"},
	Executables: []string{"bin/Runner.Listener"},
}

// fakeProcfs writes a directory shaped like /proc -- one numbered directory
// per process, each holding a NUL-separated cmdline -- and returns its path.
// Scanner.Root points at it, so an example can show exactly what Find returns
// instead of depending on whatever happens to be running.
func fakeProcfs(processes map[int][]string) string {
	root, err := os.MkdirTemp("", "procfind-example-")
	if err != nil {
		log.Fatal(err)
	}
	for pid, argv := range processes {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.Mkdir(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		cmdline := strings.Join(argv, "\x00") + "\x00"
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	return root
}

// The common case: ask the process table which pids belong to a directory.
//
// Scripts come back first. That ordering is the point of keeping Scripts and
// Executables in separate lists: a supervisor script typically restarts its
// worker in a loop, so a caller shutting the whole thing down has to signal
// the script first -- signal the worker first and the loop cheerfully starts
// another one.
func Example() {
	proc := fakeProcfs(map[int][]string{
		140: {runners + "/bin/Runner.Listener", "run"}, // the worker
		101: {"/bin/bash", runners + "/run.sh"},        // the supervising script
		12:  {"/usr/sbin/sshd", "-D"},                  // someone else entirely
	})
	defer func() { _ = os.RemoveAll(proc) }()

	scanner := procfind.Scanner{Root: proc}
	fmt.Println(scanner.Find(runners, spec))
	fmt.Println(scanner.Running(runners, spec))

	// Output:
	// [101 140]
	// true
}

// A script's argv depends on how it was started, and both forms are matched.
// Through its shebang it shows up as the interpreter running the script, so
// the script path is argv[1] and argv[0] is a shell; exec'd directly it is
// argv[0] itself. Shells are compared by base name, because bash lives in
// /bin on some distributions and /usr/bin on others.
func ExampleScanner_Find_scriptForms() {
	proc := fakeProcfs(map[int][]string{
		7:  {"/usr/bin/bash", runners + "/run.sh"},
		8:  {runners + "/run-helper.sh"},
		9:  {"/bin/sh", runners + "/run.sh"},
		20: {"python3", runners + "/run.sh"}, // not a shell: argv[1] is data
	})
	defer func() { _ = os.RemoveAll(proc) }()

	fmt.Println(procfind.Scanner{Root: proc}.Find(runners, spec))

	// Output:
	// [7 8 9]
}

// Only argv[0], and argv[1] when argv[0] is a shell, are considered. Scanning
// the whole command line would match commands that merely mention the path --
// which an operator is quite likely to be running at exactly the moment they
// are debugging why the supervisor keeps restarting things.
func ExampleScanner_Find_mentionsAreNotMatches() {
	proc := fakeProcfs(map[int][]string{
		31: {"grep", "-n", "token", runners + "/run.sh"},
		32: {"tail", "-f", runners + "/run.sh"},
		33: {"vim", runners + "/run.sh"},
	})
	defer func() { _ = os.RemoveAll(proc) }()

	fmt.Println(procfind.Scanner{Root: proc}.Find(runners, spec))

	// Output:
	// []
}

// FindMany is the primitive and Find the wrapper: the process table is read
// once no matter how many directories are asked about. A caller listing the
// state of N directories would otherwise read every entry in /proc N times,
// which on a busy host is the most expensive thing it does.
//
// Results are keyed by the exact strings passed in, so two spellings of the
// same directory each get an entry holding the same pids. Directories with
// nothing running are absent rather than empty.
func ExampleScanner_FindMany() {
	proc := fakeProcfs(map[int][]string{
		101: {"/bin/bash", runners + "/run.sh"},
		202: {"/bin/bash", "/srv/runners/two/run.sh"},
	})
	defer func() { _ = os.RemoveAll(proc) }()

	found := procfind.Scanner{Root: proc}.FindMany(
		[]string{runners, runners + "/", "/srv/runners/two", "/srv/runners/three"},
		spec,
	)
	fmt.Println(found)

	// Output:
	// map[/srv/runners/one:[101] /srv/runners/one/:[101] /srv/runners/two:[202]]
}

// Without a procfs there is nothing to read, and every lookup reports "not
// found" rather than guessing. That is also what a caller sees on Windows.
func ExampleScanner_Find_noProcfs() {
	scanner := procfind.Scanner{Root: filepath.Join(os.TempDir(), "procfind-no-such-procfs")}

	fmt.Println(scanner.Find(runners, spec))
	fmt.Println(scanner.Running(runners, spec))

	// Output:
	// []
	// false
}
