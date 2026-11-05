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
		return gitConfigGet("", "--path", "--global", "--get", "core.excludesFile")
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
		{name: "relative value: from the work tree, not the cwd", setup: func(t *testing.T, home, repo string) {
			runGit(t, repo, "config", "core.excludesFile", "rel.ign")
			writeTree(t, home, map[string]string{"repo/rel.ign": "*.local\n", "rel.ign": "*.glob\n"})
			t.Chdir(home)
		}, want: "a.local", src: "repo/rel.ign"},
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

// repoLoaderVerdicts reports, for each repo loader, whether rel (a file under
// repo) is ignored: by Match or Check, or by a walker not yielding it.
func repoLoaderVerdicts(t *testing.T, repo, rel string) map[string]bool {
	t.Helper()
	target := filepath.Join(repo, filepath.FromSlash(rel))
	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	tr, err := OpenTree(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("OpenTree: %v", err)
	}
	res, err := tr.Check(rel)
	if err != nil {
		t.Fatalf("Check(%q): %v", rel, err)
	}
	walkIgnored := true
	err = WalkRepo(repo, MatcherOptions{}, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == target {
			walkIgnored = false
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	filesIgnored := true
	for p, err := range RepoFiles(repo, MatcherOptions{}) {
		if err != nil {
			t.Fatalf("RepoFiles: %v", err)
		}
		if p == target {
			filesIgnored = false
		}
	}
	entriesIgnored := true
	for e, err := range RepoEntries(repo, MatcherOptions{}, WalkOptions{}) {
		if err != nil {
			t.Fatalf("RepoEntries: %v", err)
		}
		if e.Path == target {
			entriesIgnored = false
		}
	}
	return map[string]bool{
		"LoadRepo":    m.Match(rel, false),
		"OpenTree":    res.Ignored,
		"WalkRepo":    walkIgnored,
		"RepoFiles":   filesIgnored,
		"RepoEntries": entriesIgnored,
	}
}

// Every repo loader folds case when core.ignorecase is true, as git does,
// and not when it is false. false is set explicitly: git init writes true on
// macOS and Windows.
func TestRepoLoaders_FollowCoreIgnorecase(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			home := isolateGitConfig(t)
			runGit(t, home, "init", "-q", "repo")
			repo := filepath.Join(home, "repo")
			runGit(t, repo, "config", "core.ignorecase", value)
			writeTree(t, home, map[string]string{"repo/.gitignore": "*.log\n", "repo/A.LOG": ""})

			want := value == "true"
			if git := gitIgnores(t, repo, "A.LOG"); git != want {
				t.Fatalf("git ignores A.LOG = %v, want %v", git, want)
			}
			for name, ignored := range repoLoaderVerdicts(t, repo, "A.LOG") {
				if ignored != want {
					t.Errorf("%s: A.LOG ignored = %v, want %v", name, ignored, want)
				}
			}
		})
	}
}

// Without a repository or any config, matching stays case-sensitive.
func TestLoadRepo_NoRepoIsCaseSensitive(t *testing.T) {
	home := isolateGitConfig(t)
	writeTree(t, home, map[string]string{"repo/.gitignore": "*.log\n"})
	m, err := LoadRepo(filepath.Join(home, "repo"), MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if m.Match("A.LOG", false) {
		t.Error("A.LOG ignored by *.log with no core.ignorecase; want case-sensitive")
	}
}

// CaseInsensitive in the options still applies when core.ignorecase is false.
func TestLoadRepo_CaseInsensitiveOptionWins(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	repo := filepath.Join(home, "repo")
	runGit(t, repo, "config", "core.ignorecase", "false")
	writeTree(t, home, map[string]string{"repo/.gitignore": "*.log\n"})
	m, err := LoadRepo(repo, MatcherOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if !m.Match("A.LOG", false) {
		t.Error("A.LOG not ignored with CaseInsensitive: true; want the option to apply")
	}
}

// core.ignorecase is read from every scope, as git does, and still from the
// user's config when git rejects the repository.
func TestLoadRepo_GlobalCoreIgnorecase(t *testing.T) {
	for _, name := range []string{"no repository", "rejected repository"} {
		t.Run(name, func(t *testing.T) {
			home := isolateGitConfig(t)
			files := map[string]string{
				"global.cfg":      "[core]\n\tignorecase = true\n",
				"repo/.gitignore": "*.log\n",
			}
			if name == "rejected repository" {
				files["repo/.git"] = "not a pointer\n"
			}
			writeTree(t, home, files)
			m, err := LoadRepo(filepath.Join(home, "repo"), MatcherOptions{})
			if err != nil {
				t.Fatalf("LoadRepo: %v", err)
			}
			if !m.Match("A.LOG", false) {
				t.Error("A.LOG not ignored; want the global core.ignorecase applied")
			}
		})
	}
}

// git dies on a core.ignorecase it can't parse, so the repo loaders fail.
func TestRepoLoaders_BadCoreIgnorecase(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	repo := filepath.Join(home, "repo")
	runGit(t, repo, "config", "core.ignorecase", "maybe")

	m, err := LoadRepo(repo, MatcherOptions{})
	if m != nil || err == nil || !strings.Contains(err.Error(), "core.ignorecase") {
		t.Errorf("LoadRepo = %v, %v; want nil and an error naming core.ignorecase", m, err)
	}
	tr, err := OpenTree(repo, MatcherOptions{})
	if tr != nil || err == nil || !strings.Contains(err.Error(), "core.ignorecase") {
		t.Errorf("OpenTree = %v, %v; want nil and an error naming core.ignorecase", tr, err)
	}
}

// Without git, core.ignorecase can't be read and counts as false.
func TestLoadRepo_NoGitIsCaseSensitive(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	repo := filepath.Join(home, "repo")
	runGit(t, repo, "config", "core.ignorecase", "true")
	writeTree(t, home, map[string]string{"repo/.gitignore": "*.log\n"})
	t.Setenv("PATH", t.TempDir())

	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo without git: %v", err)
	}
	if m.Match("A.LOG", false) {
		t.Error("A.LOG ignored without git; want core.ignorecase treated as unset")
	}
}
