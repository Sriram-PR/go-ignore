package ignore_test

import (
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	ignore "github.com/Sriram-PR/go-ignore"
	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

const (
	fuzzMaxPaths    = 64
	fuzzMaxSegments = 16
)

// wellFormed reports whether name is a relative slash path with no empty,
// "." or ".." segment and no NUL, which no filesystem allows and Match
// answers as no match. Other bytes, invalid UTF-8 included, are allowed.
func wellFormed(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return false
	}
	for seg := range strings.SplitSeq(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// scenarioFromFuzz decodes FuzzTreeWalkMatch's input: root is the root
// .gitignore, sub the .gitignore in subDir, paths one path per line with a
// trailing "/" for a directory. Unusable path entries are dropped; false
// means the rest can't form a tree the walker reads without error.
func scenarioFromFuzz(root, sub, subDir, paths string) (paritycases.Scenario, bool) {
	s := paritycases.Scenario{Name: "fuzz", Files: map[string]string{}, Paths: map[string]bool{}}
	if root != "" {
		s.Files[".gitignore"] = root
	}
	conflict := false
	mark := func(p string, dir bool) {
		if old, seen := s.Paths[p]; seen && old != dir {
			conflict = true
		}
		s.Paths[p] = dir
	}
	markDirs := func(p string) {
		for i := range len(p) {
			if p[i] == '/' {
				mark(p[:i], true)
			}
		}
		mark(p, true)
	}
	if subDir != "" {
		if !wellFormed(subDir) {
			return s, false
		}
		markDirs(subDir)
		if sub != "" {
			s.Files[subDir+"/.gitignore"] = sub
		}
	}
	kept := 0
	for e := range strings.SplitSeq(paths, "\n") {
		if kept == fuzzMaxPaths {
			break
		}
		dir := strings.HasSuffix(e, "/")
		e = strings.TrimSuffix(e, "/")
		if !wellFormed(e) || strings.Count(e, "/") >= fuzzMaxSegments || path.Base(e) == ".gitignore" {
			continue
		}
		kept++
		if i := strings.LastIndexByte(e, '/'); i >= 0 {
			markDirs(e[:i])
		}
		mark(e, dir)
	}
	if conflict || kept == 0 {
		return s, false
	}
	for p, dir := range s.Paths {
		// fstest.MapFS can't open a directory whose name fs.ValidPath
		// rejects, and the walker reports a non-file .gitignore as an error.
		if dir && (!fs.ValidPath(p) || path.Base(p) == ".gitignore") {
			return s, false
		}
		if runtime.GOOS == "windows" && strings.Contains(p, `\`) {
			return s, false
		}
	}
	return s, true
}

// fuzzSeedArgs is s as FuzzTreeWalkMatch's input, or false when s is
// case-insensitive or has more .gitignore files than root and one nested.
func fuzzSeedArgs(s paritycases.Scenario) (root, sub, subDir, paths string, ok bool) {
	if s.ICase {
		return "", "", "", "", false
	}
	for f, c := range s.Files {
		switch {
		case f == ".gitignore":
			root = c
		case path.Base(f) != ".gitignore" || subDir != "":
			return "", "", "", "", false
		default:
			subDir, sub = path.Dir(f), c
		}
	}
	lines := make([]string, 0, len(s.Paths))
	for _, p := range slices.Sorted(maps.Keys(s.Paths)) {
		if s.Paths[p] {
			p += "/"
		}
		lines = append(lines, p)
	}
	return root, sub, subDir, strings.Join(lines, "\n"), true
}

func TestScenarioFromFuzz(t *testing.T) {
	many := make([]string, 0, 70)
	manyWant := map[string]bool{}
	for i := range 70 {
		many = append(many, fmt.Sprintf("f%d", i))
		if i < 64 {
			manyWant[fmt.Sprintf("f%d", i)] = false
		}
	}
	deep := strings.Repeat("d/", 16) + "f" // 17 segments

	type want struct {
		files map[string]string
		paths map[string]bool
	}
	tests := []struct {
		name                     string
		root, sub, subDir, paths string
		ok                       bool
		want                     want
	}{
		{name: "dir marker and prefixes", paths: "a/b\nc/", ok: true,
			want: want{map[string]string{}, map[string]bool{"a": true, "a/b": false, "c": true}}},
		{name: "root and sub", root: "x", sub: "y", subDir: "d/e", paths: "f", ok: true,
			want: want{map[string]string{".gitignore": "x", "d/e/.gitignore": "y"}, map[string]bool{"d": true, "d/e": true, "f": false}}},
		{name: "sub unused without subDir", sub: "y", paths: "f", ok: true,
			want: want{map[string]string{}, map[string]bool{"f": false}}},
		{name: "subDir without sub is still a dir", subDir: "d", paths: "f", ok: true,
			want: want{map[string]string{}, map[string]bool{"d": true, "f": false}}},
		{name: "drops", paths: "\n.\na/../b\n/a\nx/.gitignore\na\x00b\n" + deep + "\nkeep", ok: true,
			want: want{map[string]string{}, map[string]bool{"keep": false}}},
		{name: "non-UTF-8 file kept", paths: "d/\xff", ok: true,
			want: want{map[string]string{}, map[string]bool{"d": true, "d/\xff": false}}},
		{name: "first 64 kept", paths: strings.Join(many, "\n"), ok: true,
			want: want{map[string]string{}, manyWant}},
		{name: "subDir dot", subDir: ".", paths: "f"},
		{name: "subDir empty segment", subDir: "a//b", paths: "f"},
		{name: "subDir leading slash", subDir: "/a", paths: "f"},
		{name: "subDir non-UTF-8", subDir: "\xff", paths: "f"},
		{name: "subDir with NUL", subDir: "a\x00b", paths: "f"},
		{name: "non-UTF-8 dir prefix", paths: "\xff/x"},
		{name: "non-UTF-8 dir marker", paths: "\xff/"},
		{name: "dir named .gitignore", paths: ".gitignore/x"},
		{name: "subDir named .gitignore", subDir: "a/.gitignore", paths: "f"},
		{name: "subDir .gitignore at root", subDir: ".gitignore", paths: "f"},
		{name: "file and dir", paths: "a\na/b"},
		{name: "subDir listed as file", subDir: "a/b", paths: "a"},
		{name: "no paths kept", root: "x", paths: "\n."},
		{name: "backslash", paths: `a\b`, ok: runtime.GOOS != "windows",
			want: want{map[string]string{}, map[string]bool{`a\b`: false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := scenarioFromFuzz(tt.root, tt.sub, tt.subDir, tt.paths)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (scenario %+v)", ok, tt.ok, s)
			}
			if !ok {
				return
			}
			if !maps.Equal(s.Files, tt.want.files) || !maps.Equal(s.Paths, tt.want.paths) {
				t.Errorf("Files %q Paths %v, want %q %v", s.Files, s.Paths, tt.want.files, tt.want.paths)
			}
			if s.Name != "fuzz" || s.ICase {
				t.Errorf("Name %q ICase %v, want fuzz false", s.Name, s.ICase)
			}
		})
	}
}

// Every seed decodes back to its curated scenario, so seeding loses nothing.
func TestFuzzSeedsRoundTrip(t *testing.T) {
	nonEmpty := func(m map[string]string) map[string]string {
		m = maps.Clone(m)
		maps.DeleteFunc(m, func(_, c string) bool { return c == "" })
		return m
	}
	n := 0
	for _, s := range paritycases.Curated() {
		root, sub, subDir, paths, ok := fuzzSeedArgs(s)
		if !ok {
			continue
		}
		if runtime.GOOS == "windows" && strings.Contains(subDir+paths, `\`) {
			continue
		}
		n++
		got, ok := scenarioFromFuzz(root, sub, subDir, paths)
		if !ok {
			t.Errorf("%s: decoder rejects the seed", s.Name)
			continue
		}
		if !maps.Equal(nonEmpty(got.Files), nonEmpty(s.Files)) || !maps.Equal(got.Paths, s.Paths) {
			t.Errorf("%s: decoded Files %q Paths %v, want %q %v", s.Name, got.Files, got.Paths, s.Files, s.Paths)
		}
	}
	if n == 0 {
		t.Fatal("no curated scenario fits the fuzzer")
	}
}

// FuzzTreeWalkMatch checks that a preloaded Matcher, Tree.Check and FilesFS
// give one answer on scenarios decoded from the input.
func FuzzTreeWalkMatch(f *testing.F) {
	for _, s := range paritycases.Curated() {
		if root, sub, subDir, paths, ok := fuzzSeedArgs(s); ok {
			f.Add(root, sub, subDir, paths)
		}
	}
	f.Fuzz(func(t *testing.T, root, sub, subDir, paths string) {
		s, ok := scenarioFromFuzz(root, sub, subDir, paths)
		if !ok {
			t.Skip()
		}
		checkWalkAgreesWithMatch(t, s)
		checkTreeAgreesWithMatcher(t, s)
	})
}

func TestGitParitySkip(t *testing.T) {
	for p, skip := range map[string]bool{
		":x":      true,
		":(top)a": true,
		"x/:a":    false,
		"a":       false,
		"a:b":     false,
	} {
		if got := gitParitySkip(p) != ""; got != skip {
			t.Errorf("gitParitySkip(%q) skips = %v, want %v", p, got, skip)
		}
	}
}

// gitParitySkip returns why path p is left out of the git comparison, or "".
// A reason starting with an issue number is a known go-ignore bug; any other
// is a git or harness quirk no go-ignore change can fix.
func gitParitySkip(p string) string {
	if strings.HasPrefix(p, ":") {
		return "check-ignore reads a leading ':' as pathspec magic and refuses literal pathspecs"
	}
	return ""
}

// nameMax is Linux's NAME_MAX: Materialize can't create a longer name.
const nameMax = 255

// checkGitParity writes s to a fresh git repository and checks that Match
// gives git check-ignore's verdict for every path, and Tree.Check its
// verdict, source and line.
func checkGitParity(t *testing.T, s paritycases.Scenario) {
	t.Helper()
	for _, p := range slices.Concat(slices.Collect(maps.Keys(s.Paths)), slices.Collect(maps.Keys(s.Files))) {
		for seg := range strings.SplitSeq(p, "/") {
			if len(seg) > nameMax {
				t.Skip("a name is longer than NAME_MAX")
			}
		}
	}
	dir := t.TempDir()
	if err := paritycases.InitRepo(dir, s); err != nil {
		t.Fatal(err)
	}
	isDir, err := paritycases.Materialize(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, p := range slices.Sorted(maps.Keys(isDir)) {
		if path.Base(p) != ".gitignore" && gitParitySkip(p) == "" {
			paths = append(paths, p)
		}
	}
	git, err := paritycases.GitVerdicts(dir, s, paths)
	if err != nil {
		t.Fatal(err)
	}
	m := parityMatcher(s)
	tr := ignore.NewWithOptions(parityOptions(s)).Tree(dir)
	for _, p := range paths {
		g, ok := git[p]
		if !ok {
			t.Errorf("git did not report %q\nfiles: %q", p, s.Files)
			continue
		}
		if lib := m.MatchWithReason(p, isDir[p]); lib.Ignored != g.Ignored {
			t.Errorf("%q dir=%v: git ignored=%v (%s:%d), Match %+v\nfiles: %q", p, isDir[p], g.Ignored, g.Source, g.Line, lib, s.Files)
			continue
		}
		res, err := tr.Check(p)
		if err != nil {
			t.Errorf("Check(%q): %v", p, err)
			continue
		}
		src := ""
		if res.Matched {
			rel, err := filepath.Rel(dir, res.Source)
			if err != nil {
				t.Fatal(err)
			}
			src = filepath.ToSlash(rel)
		}
		if res.Ignored != g.Ignored || src != g.Source || res.Line != g.Line {
			t.Errorf("%q dir=%v: git ignored=%v %s:%d, Tree ignored=%v %s:%d\nfiles: %q", p, isDir[p], g.Ignored, g.Source, g.Line, res.Ignored, src, res.Line, s.Files)
		}
	}
}

// FuzzGitParity compares go-ignore with git check-ignore on scenarios decoded
// from the input. Runs only with GOIGNORE_PARITY=1.
func FuzzGitParity(f *testing.F) {
	requireParity(f)
	for _, s := range paritycases.Curated() {
		if root, sub, subDir, paths, ok := fuzzSeedArgs(s); ok {
			f.Add(root, sub, subDir, paths)
		}
	}
	f.Fuzz(func(t *testing.T, root, sub, subDir, paths string) {
		s, ok := scenarioFromFuzz(root, sub, subDir, paths)
		if !ok {
			t.Skip()
		}
		checkGitParity(t, s)
	})
}
