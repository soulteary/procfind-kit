package procfind

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeProc builds a procfs-shaped fixture: one directory per pid, each with a
// cmdline file holding NUL-separated argv.
func fakeProc(t *testing.T, procs map[int][]string) string {
	t.Helper()
	root := t.TempDir()
	for pid, argv := range procs {
		dir := filepath.Join(root, itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		var buf []byte
		for _, a := range argv {
			buf = append(buf, []byte(a)...)
			buf = append(buf, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

var spec = Spec{
	Scripts:     []string{"run.sh", "run-helper.sh"},
	Executables: []string{"bin/worker"},
}

func TestFindsExecutableByArgv0(t *testing.T) {
	dir := "/srv/app"
	root := fakeProc(t, map[int][]string{
		42: {filepath.Join(dir, "bin", "worker"), "run"},
	})
	got := Scanner{Root: root}.Find(dir, spec)
	if !reflect.DeepEqual(got, []int{42}) {
		t.Fatalf("Find = %v, want [42]", got)
	}
}

// A script launched through its shebang shows up as the interpreter running
// the script: the path is argv[1], not argv[0].
func TestFindsScriptBehindShebang(t *testing.T) {
	dir := "/srv/app"
	root := fakeProc(t, map[int][]string{
		7: {"/bin/bash", filepath.Join(dir, "run.sh")},
	})
	got := Scanner{Root: root}.Find(dir, spec)
	if !reflect.DeepEqual(got, []int{7}) {
		t.Fatalf("Find = %v, want [7]", got)
	}
}

func TestFindsScriptExecedDirectly(t *testing.T) {
	dir := "/srv/app"
	root := fakeProc(t, map[int][]string{
		9: {filepath.Join(dir, "run.sh")},
	})
	if got := (Scanner{Root: root}).Find(dir, spec); !reflect.DeepEqual(got, []int{9}) {
		t.Fatalf("Find = %v, want [9]", got)
	}
}

// The reason argv[1] is only honoured when argv[0] is a shell: an operator
// debugging the very problem this package solves is likely to be running one
// of these, and counting it as "the service is up" would hide the outage.
func TestIgnoresCommandsThatMerelyMentionThePath(t *testing.T) {
	dir := "/srv/app"
	script := filepath.Join(dir, "run.sh")
	for name, argv := range map[string][]string{
		"grep": {"/usr/bin/grep", script},
		"tail": {"/usr/bin/tail", script},
		"vim":  {"/usr/bin/vim", script},
		"cat":  {"/bin/cat", script},
	} {
		t.Run(name, func(t *testing.T) {
			root := fakeProc(t, map[int][]string{100: argv})
			if got := (Scanner{Root: root}).Find(dir, spec); len(got) != 0 {
				t.Fatalf("Find = %v, want none", got)
			}
		})
	}
}

// Scripts come first so a caller can stop the supervisor before its worker;
// the other way round, the supervisor's restart loop just starts another one.
func TestScriptsSortBeforeExecutables(t *testing.T) {
	dir := "/srv/app"
	root := fakeProc(t, map[int][]string{
		10: {filepath.Join(dir, "bin", "worker")},
		20: {"/bin/bash", filepath.Join(dir, "run.sh")},
		30: {"/bin/sh", filepath.Join(dir, "run-helper.sh")},
	})
	got := Scanner{Root: root}.Find(dir, spec)
	want := []int{20, 30, 10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Find = %v, want %v (scripts ascending, then executables)", got, want)
	}
}

func TestFindManyKeysByTheStringsGivenIn(t *testing.T) {
	root := fakeProc(t, map[int][]string{
		1: {"/bin/bash", "/srv/a/run.sh"},
		2: {"/bin/bash", "/srv/b/run.sh"},
	})
	got := Scanner{Root: root}.FindMany([]string{"/srv/a", "/srv/b", "/srv/c"}, spec)
	if !reflect.DeepEqual(got["/srv/a"], []int{1}) || !reflect.DeepEqual(got["/srv/b"], []int{2}) {
		t.Fatalf("FindMany = %v", got)
	}
	if _, ok := got["/srv/c"]; ok {
		t.Fatalf("a directory with no processes should be absent, got %v", got["/srv/c"])
	}
}

// Two spellings of the same directory are one target internally -- otherwise a
// process would be claimed by whichever spelling came first -- but each still
// gets its own entry in the result.
func TestFindManyGivesEverySpellingTheSamePids(t *testing.T) {
	root := fakeProc(t, map[int][]string{
		5: {"/bin/bash", "/srv/a/run.sh"},
	})
	got := Scanner{Root: root}.FindMany([]string{"/srv/a", "/srv/a/", "/srv/a/."}, spec)
	for _, key := range []string{"/srv/a", "/srv/a/", "/srv/a/."} {
		if !reflect.DeepEqual(got[key], []int{5}) {
			t.Fatalf("FindMany[%q] = %v, want [5]", key, got[key])
		}
	}
}

func TestSkipsKernelThreadsAndUnreadableEntries(t *testing.T) {
	root := fakeProc(t, map[int][]string{
		3: {}, // kernel thread: empty cmdline
		4: {"/bin/bash", "/srv/app/run.sh"},
	})
	// A non-numeric entry, as /proc is full of.
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := (Scanner{Root: root}).Find("/srv/app", spec); !reflect.DeepEqual(got, []int{4}) {
		t.Fatalf("Find = %v, want [4]", got)
	}
}

// Without a procfs there is no way to answer, and "not found" is the honest
// answer -- not a guess in either direction.
func TestNoProcfsReportsNotFound(t *testing.T) {
	s := Scanner{Root: filepath.Join(t.TempDir(), "does-not-exist")}
	if got := s.Find("/srv/app", spec); got != nil {
		t.Fatalf("Find = %v, want nil", got)
	}
	if s.Running("/srv/app", spec) {
		t.Fatal("Running = true without a procfs")
	}
}

func TestRelativeDirectoryIsResolved(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := fakeProc(t, map[int][]string{
		11: {filepath.Join(wd, "app", "bin", "worker")},
	})
	if got := (Scanner{Root: root}).Find("app", spec); !reflect.DeepEqual(got, []int{11}) {
		t.Fatalf("Find = %v, want [11]", got)
	}
}

func TestEmptyInputs(t *testing.T) {
	root := fakeProc(t, map[int][]string{1: {"/bin/bash", "/srv/a/run.sh"}})
	s := Scanner{Root: root}
	if got := s.FindMany(nil, spec); got != nil {
		t.Fatalf("FindMany(nil) = %v, want nil", got)
	}
	if got := s.FindMany([]string{""}, spec); got != nil {
		t.Fatalf("FindMany([\"\"]) = %v, want nil", got)
	}
	// An empty Spec matches nothing, rather than everything.
	if got := s.Find("/srv/a", Spec{}); len(got) != 0 {
		t.Fatalf("Find with empty Spec = %v, want none", got)
	}
}

func TestRunning(t *testing.T) {
	root := fakeProc(t, map[int][]string{1: {"/bin/bash", "/srv/a/run.sh"}})
	s := Scanner{Root: root}
	if !s.Running("/srv/a", spec) {
		t.Fatal("Running(/srv/a) = false, want true")
	}
	if s.Running("/srv/b", spec) {
		t.Fatal("Running(/srv/b) = true, want false")
	}
}

func TestIsShellComparesBaseNameOnly(t *testing.T) {
	for _, path := range []string{"/bin/bash", "/usr/bin/bash", "bash", "/bin/sh", "/bin/busybox"} {
		if !isShell(path) {
			t.Errorf("isShell(%q) = false", path)
		}
	}
	for _, path := range []string{"/usr/bin/python3", "/bin/cat", "bashful", ""} {
		if isShell(path) {
			t.Errorf("isShell(%q) = true", path)
		}
	}
}

// The package-level helpers read the real /proc. They must not panic there,
// and on a host running this test suite nothing lives under the fake path.
func TestPackageLevelHelpersUseRealProc(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nothing-here")
	if got := Find(dir, spec); len(got) != 0 {
		t.Fatalf("Find = %v, want none", got)
	}
	if Running(dir, spec) {
		t.Fatal("Running = true for a directory with no processes")
	}
	if got := FindMany([]string{dir}, spec); len(got) != 0 {
		t.Fatalf("FindMany = %v, want empty", got)
	}
}
