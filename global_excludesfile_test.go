package ignore

import (
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitConfigGet maps git's exit status: 0 is a value (possibly empty), 1 is
// unset, anything else an error carrying git's message.
func TestGitConfigGet_Outcomes(t *testing.T) {
	home := isolateGitConfig(t)
	get := func() (string, bool, error) {
		return gitConfigGet("", "--global", "--get", "core.excludesFile")
	}

	if v, found, err := get(); err != nil || found {
		t.Errorf("unset: got (%q, %v, %v), want not found", v, found, err)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile =\n"})
	if v, found, err := get(); err != nil || !found || v != "" {
		t.Errorf("empty: got (%q, %v, %v), want (\"\", true, nil)", v, found, err)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile = ~/x.ign\n"})
	want := filepath.Join(home, "x.ign")
	if v, found, err := get(); err != nil || !found || filepath.Clean(v) != want {
		t.Errorf("tilde: got (%q, %v, %v), want %q", v, found, err, want)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core\n"})
	_, found, err := get()
	var exitErr *exec.ExitError
	if found || !errors.As(err, &exitErr) || !strings.Contains(err.Error(), "bad config") {
		t.Errorf("malformed: got (%v, %v), want an *exec.ExitError mentioning bad config", found, err)
	}
}

// The repo loaders load the excludes file git uses: for every probe file,
// LoadRepo agrees with git check-ignore, and the case's expected file is the
// rule's Source.
func TestLoadRepo_ExcludesFileMatchesGit(t *testing.T) {
	probes := []string{"a.sys", "a.glob", "a.local", "a.inc", "a.xdg", "a.env"}
	cases := []struct {
		name        string
		skipWindows string
		setup       func(t *testing.T, home, repo string)
		want        string // the one probe ignored, "" for none
		src         string // the rule's file, relative to home
	}{
		{name: "system and global: global wins", setup: func(t *testing.T, home, repo string) {
			setExcludes(t, filepath.Join(home, "system.cfg"), filepath.Join(home, "sys.ign"))
			setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "glob.ign"))
		}, want: "a.glob", src: "glob.ign"},
		{name: "repo config overrides global", setup: func(t *testing.T, home, repo string) {
			setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "glob.ign"))
			runGit(t, repo, "config", "core.excludesFile", gitPath(filepath.Join(home, "local.ign")))
		}, want: "a.local", src: "local.ign"},
		{name: "include after the value overrides it", setup: func(t *testing.T, home, repo string) {
			setExcludes(t, filepath.Join(home, "inc.cfg"), filepath.Join(home, "inc.ign"))
			writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile = " +
				gitPath(filepath.Join(home, "glob.ign")) + "\n[include]\n\tpath = " +
				gitPath(filepath.Join(home, "inc.cfg")) + "\n"})
		}, want: "a.inc", src: "inc.ign"},
		{name: "includeIf gitdir", skipWindows: "temp paths may be 8.3 short names that gitdir: does not match",
			setup: func(t *testing.T, home, repo string) {
				resolved, err := filepath.EvalSymlinks(repo)
				if err != nil {
					t.Fatal(err)
				}
				setExcludes(t, filepath.Join(home, "inc.cfg"), filepath.Join(home, "inc.ign"))
				writeTree(t, home, map[string]string{"global.cfg": "[includeIf \"gitdir:" +
					gitPath(resolved) + "/\"]\n\tpath = " + gitPath(filepath.Join(home, "inc.cfg")) + "\n"})
			}, want: "a.inc", src: "inc.ign"},
		{name: "set to a missing file: no XDG fallback", setup: func(t *testing.T, home, repo string) {
			setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "missing.ign"))
		}},
		{name: "set to empty: nothing loaded", setup: func(t *testing.T, home, repo string) {
			writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile =\n"})
		}},
		{name: "unset: XDG default", setup: func(t *testing.T, home, repo string) {},
			want: "a.xdg", src: ".config/git/ignore"},
		{name: "tilde path", setup: func(t *testing.T, home, repo string) {
			writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile = ~/glob.ign\n"})
		}, want: "a.glob", src: "glob.ign"},
		{name: "GIT_CONFIG_COUNT override wins", setup: func(t *testing.T, home, repo string) {
			setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "glob.ign"))
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.excludesFile")
			t.Setenv("GIT_CONFIG_VALUE_0", filepath.Join(home, "env.ign"))
		}, want: "a.env", src: "env.ign"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipWindows != "" && runtime.GOOS == "windows" {
				t.Skip(tc.skipWindows)
			}
			home := isolateGitConfig(t)
			runGit(t, home, "init", "-q", "repo")
			repo := filepath.Join(home, "repo")
			files := map[string]string{
				"sys.ign":            "*.sys\n",
				"glob.ign":           "*.glob\n",
				"local.ign":          "*.local\n",
				"inc.ign":            "*.inc\n",
				"env.ign":            "*.env\n",
				".config/git/ignore": "*.xdg\n",
			}
			for _, p := range probes {
				files["repo/"+p] = ""
			}
			writeTree(t, home, files)
			tc.setup(t, home, repo)

			m, err := LoadRepo(repo, MatcherOptions{})
			if err != nil {
				t.Fatalf("LoadRepo: %v", err)
			}
			for _, p := range probes {
				got, git := m.Match(p, false), gitIgnores(t, repo, p)
				if got != git {
					t.Errorf("Match(%q) = %v, git check-ignore says %v", p, got, git)
				}
				if git != (p == tc.want) {
					t.Errorf("git ignores %q = %v; the case expects only %q ignored", p, git, tc.want)
				}
			}
			if tc.want != "" {
				r := m.MatchWithReason(tc.want, false)
				if want := filepath.Join(home, filepath.FromSlash(tc.src)); filepath.Clean(r.Source) != want {
					t.Errorf("Source of %q = %q, want %q", tc.want, r.Source, want)
				}
			}
		})
	}
}

// An inherited GIT_DIR must not redirect LoadRepo to another repository's
// config: repoRoot decides.
func TestLoadRepo_IgnoresInheritedGitDir(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	runGit(t, home, "init", "-q", "other")
	writeTree(t, home, map[string]string{"local.ign": "*.local\n", "other.ign": "*.other\n"})
	repo, other := filepath.Join(home, "repo"), filepath.Join(home, "other")
	runGit(t, repo, "config", "core.excludesFile", gitPath(filepath.Join(home, "local.ign")))
	runGit(t, other, "config", "core.excludesFile", gitPath(filepath.Join(home, "other.ign")))
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if !m.Match("a.local", false) || m.Match("a.other", false) {
		t.Errorf("a.local ignored = %v, a.other ignored = %v; want repo's rules only",
			m.Match("a.local", false), m.Match("a.other", false))
	}
}

// git refuses to run with a malformed gitconfig, so the repo loaders fail
// instead of guessing.
func TestRepoLoaders_MalformedGitconfigErrors(t *testing.T) {
	home := isolateGitConfig(t)
	writeTree(t, home, map[string]string{"global.cfg": "[core\n", "repo/a.txt": "x"})
	repo := filepath.Join(home, "repo")

	if _, err := LoadRepo(repo, MatcherOptions{}); err == nil || !strings.Contains(err.Error(), "bad config") {
		t.Errorf("LoadRepo err = %v, want a bad config error", err)
	}
	if _, err := OpenTree(repo, MatcherOptions{}); err == nil || !strings.Contains(err.Error(), "bad config") {
		t.Errorf("OpenTree err = %v, want a bad config error", err)
	}

	calls := 0
	err := WalkRepo(repo, MatcherOptions{}, func(string, fs.DirEntry, error) error {
		calls++
		return nil
	})
	if err == nil || calls != 0 {
		t.Errorf("WalkRepo: err = %v after %d fn calls, want an error and no calls", err, calls)
	}

	yields, errs := 0, 0
	for _, err := range RepoFiles(repo, MatcherOptions{}) {
		yields++
		if err != nil {
			errs++
		}
	}
	if yields != 1 || errs != 1 {
		t.Errorf("RepoFiles yielded %d values with %d errors, want exactly one error", yields, errs)
	}
}

// LoadRepo on a repoRoot that does not exist succeeds with no repository
// rules, as before git config was run in repoRoot.
func TestLoadRepo_MissingRootSucceeds(t *testing.T) {
	home := isolateGitConfig(t)
	if _, err := LoadRepo(filepath.Join(home, "no", "such", "repo"), MatcherOptions{}); err != nil {
		t.Errorf("LoadRepo: %v", err)
	}
}

// git refuses to run in a repository whose .git it rejects, so the repo
// loaders ask without the repository: the user's core.excludesFile still
// applies, and the XDG default stays off.
func TestLoadRepo_BrokenGitFileUsesUserConfig(t *testing.T) {
	home := isolateGitConfig(t)
	writeTree(t, home, map[string]string{
		"glob.ign":           "*.glob\n",
		".config/git/ignore": "*.xdg\n",
		"repo/.git":          "not a pointer\n",
	})
	setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "glob.ign"))

	m, err := LoadRepo(filepath.Join(home, "repo"), MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if !m.Match("a.glob", false) || m.Match("a.xdg", false) {
		t.Errorf("a.glob ignored = %v, a.xdg ignored = %v; want the global file only",
			m.Match("a.glob", false), m.Match("a.xdg", false))
	}
}

// AddGlobalPatterns follows an include in the global config.
func TestAddGlobalPatterns_FollowsInclude(t *testing.T) {
	home := isolateGitConfig(t)
	writeTree(t, home, map[string]string{
		"inc.ign":    "*.inc\n",
		"global.cfg": "[include]\n\tpath = " + gitPath(filepath.Join(home, "inc.cfg")) + "\n",
	})
	setExcludes(t, filepath.Join(home, "inc.cfg"), filepath.Join(home, "inc.ign"))
	m := New()
	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}
	if !m.Match("a.inc", false) {
		t.Error("a.inc not ignored; want the included core.excludesFile loaded")
	}
}

// A global core.excludesFile that is set, to a missing file or to the empty
// string, replaces the XDG default, as in git. (The missing-file case already
// held before includes were followed; it guards the rule.)
func TestAddGlobalPatterns_SetValueSkipsXDG(t *testing.T) {
	for _, name := range []string{"missing file", "empty"} {
		t.Run(name, func(t *testing.T) {
			home := isolateGitConfig(t)
			value := ""
			if name == "missing file" {
				value = gitPath(filepath.Join(home, "missing.ign"))
			}
			writeTree(t, home, map[string]string{
				".config/git/ignore": "*.xdg\n",
				"global.cfg":         "[core]\n\texcludesFile = " + value + "\n",
			})
			m := New()
			if err := m.AddGlobalPatterns(); err != nil {
				t.Fatalf("AddGlobalPatterns: %v", err)
			}
			if m.Match("a.xdg", false) {
				t.Error("a.xdg ignored; the XDG default must not load when the global value is set")
			}
		})
	}
}

// AddSystemPatterns follows an include in the system config.
func TestAddSystemPatterns_FollowsInclude(t *testing.T) {
	home := isolateGitConfig(t)
	writeTree(t, home, map[string]string{
		"inc.ign":    "*.inc\n",
		"system.cfg": "[include]\n\tpath = " + gitPath(filepath.Join(home, "inc.cfg")) + "\n",
	})
	setExcludes(t, filepath.Join(home, "inc.cfg"), filepath.Join(home, "inc.ign"))
	m := New()
	if err := m.AddSystemPatterns(); err != nil {
		t.Fatalf("AddSystemPatterns: %v", err)
	}
	if !m.Match("a.inc", false) {
		t.Error("a.inc not ignored; want the included core.excludesFile loaded")
	}
}
