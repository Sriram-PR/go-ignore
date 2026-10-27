package ignore_test

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// An override decides over every ignore file: it ignores what a nested "!"
// re-includes, and its own "!" re-includes what .gitignore ignores, but not
// below a directory that stays ignored.
func TestOverrides_Precedence(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":    {Data: []byte("*.txt\nbuild/\n")},
		"a/.gitignore":  {Data: []byte("!secret.txt\n")},
		"a/secret.txt":  {},
		"build/out.txt": {},
		"build/app":     {},
		"dist/out.txt":  {},
	}
	base := []string{".gitignore", "a/.gitignore", "a/secret.txt"}
	cases := []struct {
		name      string
		overrides []string
		files     []string        // FilesFS, in walk order
		ignored   map[string]bool // TreeFS Check
	}{
		{"none", nil, base,
			map[string]bool{"a/secret.txt": false, "build/app": true, "dist/out.txt": true}},
		{"exclude beats nested negation", []string{"secret.txt"}, []string{".gitignore", "a/.gitignore"},
			map[string]bool{"a/secret.txt": true}},
		{"re-include needs the directory", []string{"!build/out.txt"}, base,
			map[string]bool{"build/out.txt": true}},
		{"re-include directory and file", []string{"!build/", "!build/out.txt"},
			[]string{".gitignore", "a/.gitignore", "a/secret.txt", "build/app", "build/out.txt"},
			map[string]bool{"build/out.txt": false, "build/app": false}},
		{"re-include file", []string{"!dist/out.txt"}, append(slices.Clone(base), "dist/out.txt"),
			map[string]bool{"dist/out.txt": false}},
	}
	for _, c := range cases {
		m := ignore.NewWithOptions(ignore.MatcherOptions{Overrides: c.overrides})
		var files []string
		for p, err := range m.FilesFS(fsys, ".") {
			if err != nil {
				t.Fatalf("%s: FilesFS: %v", c.name, err)
			}
			files = append(files, p)
		}
		if !slices.Equal(files, c.files) {
			t.Errorf("%s: FilesFS = %q, want %q", c.name, files, c.files)
		}
		tr := m.TreeFS(fsys, ".")
		for p, want := range c.ignored {
			r, err := tr.Check(p)
			if err != nil || r.Ignored != want {
				t.Errorf("%s: Check(%q) = %+v, %v; want Ignored=%v", c.name, p, r, err, want)
			}
		}
	}
}

func TestOverrides_ProvenanceAndWarnings(t *testing.T) {
	var warnings []ignore.ParseWarning
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		Overrides:      []string{"# keys", "", "*.pem", "a\nb", "!keep.pem"},
		WarningHandler: func(w ignore.ParseWarning) { warnings = append(warnings, w) },
	})
	m.AddPatterns("", []byte("!*.pem\n"))
	if r := m.MatchWithReason("x.pem", false); !r.Ignored || r.Source != "overrides" || r.Line != 3 || r.Rule != "*.pem" || r.BasePath != "" {
		t.Errorf("x.pem: %+v, want ignored by overrides line 3, *.pem", r)
	}
	if r := m.MatchWithReason("keep.pem", false); r.Ignored || !r.Matched || r.Source != "overrides" || r.Line != 5 {
		t.Errorf("keep.pem: %+v, want re-included by overrides line 5", r)
	}
	if m.Match("a", false) || m.Match("b", false) {
		t.Error("an override with a line break must be dropped")
	}
	if len(warnings) != 1 || warnings[0].Line != 4 || warnings[0].Pattern != "a\nb" {
		t.Errorf("warnings = %+v, want one for line 4", warnings)
	}

	quiet := ignore.NewWithOptions(ignore.MatcherOptions{Overrides: []string{"x\r"}})
	if w := quiet.Warnings(); len(w) != 1 || w[0].Line != 1 {
		t.Errorf("Warnings() = %+v, want one for line 1", w)
	}
}

func TestOverrides_LimitsCaseAndCopy(t *testing.T) {
	lines := []string{"a", "b"}
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		Overrides:      lines,
		MaxPatterns:    2,
		WarningHandler: func(ignore.ParseWarning) {},
	})
	lines[0] = "z"
	m.AddPatterns("", []byte("c\n"))
	if !m.Match("a", false) || m.Match("z", false) || m.Match("c", false) {
		t.Error("overrides must be copied at construction and fill MaxPatterns first")
	}
	if n := m.RuleCount(); n != 2 {
		t.Errorf("RuleCount = %d, want 2", n)
	}

	icase := ignore.NewWithOptions(ignore.MatcherOptions{Overrides: []string{"*.PEM"}, CaseInsensitive: true})
	if !icase.Match("Key.pem", false) {
		t.Error("a CaseInsensitive override must match Key.pem")
	}
}

// On scenarios whose only file is the root .gitignore, moving its second
// half into Overrides changes nothing: overrides act as lines appended to the
// root file. Match, the walker and Tree are each compared with the unsplit
// scenario, which the parity harness compares with git.
func TestOverrides_AgreeWithRootFile(t *testing.T) {
	n := 0
	for _, s := range walkInvariantScenarios() {
		root, ok := s.Files[".gitignore"]
		if len(s.Files) != 1 || !ok || strings.Contains(root, "\r") || strings.HasPrefix(root, "\ufeff") {
			continue
		}
		n++
		lines := strings.Split(root, "\n")
		k := len(lines) / 2
		head := strings.Join(lines[:k], "\n")
		if k > 0 {
			head += "\n"
		}
		split := paritycases.Scenario{Name: s.Name, Files: map[string]string{".gitignore": head}, Paths: s.Paths}
		opts := parityOptions(s)
		opts.Overrides = lines[k:]

		fsys, splitFS := scenarioFS(s), scenarioFS(split)
		want := parityMatcher(s)
		got := ignore.NewWithOptions(opts)
		got.AddPatternsWithSource("", ".gitignore", []byte(head))
		wantTree := ignore.NewWithOptions(parityOptions(s)).TreeFS(fsys, ".")
		gotTree := ignore.NewWithOptions(opts).TreeFS(splitFS, ".")
		for p, f := range fsys {
			isDir := f.Mode.IsDir()
			if g, w := got.Match(p, isDir), want.Match(p, isDir); g != w {
				t.Errorf("%s: Match(%q) = %v, want %v\nroot: %q\noverrides: %q", s.Name, p, g, w, head, opts.Overrides)
			}
			gr, gerr := gotTree.Check(p)
			wr, werr := wantTree.Check(p)
			if gerr != nil || werr != nil || gr.Ignored != wr.Ignored {
				t.Errorf("%s: Check(%q) = %v, %v; unsplit %v, %v", s.Name, p, gr.Ignored, gerr, wr.Ignored, werr)
			}
		}
		gotFiles := filesSet(t, ignore.NewWithOptions(opts).FilesFS(splitFS, "."))
		wantFiles := filesSet(t, ignore.NewWithOptions(parityOptions(s)).FilesFS(fsys, "."))
		if !maps.Equal(gotFiles, wantFiles) {
			t.Errorf("%s: FilesFS = %q, want %q", s.Name, slices.Sorted(maps.Keys(gotFiles)), slices.Sorted(maps.Keys(wantFiles)))
		}
	}
	if n < 20 {
		t.Fatalf("only %d root-only scenarios; the comparison would prove little", n)
	}
}

// Construction warnings (bad IgnoreFiles names, bad overrides) are capped
// like any other call.
func TestOverrides_WarningsCapped(t *testing.T) {
	const marker = "warning limit reached, further warnings dropped"
	opts := ignore.MatcherOptions{
		MaxWarnings: 3,
		IgnoreFiles: []string{"a/b", "c/d"},
		Overrides:   []string{"x\ny", "x\ny", "x\ny", "x\ny"},
	}
	if w := ignore.NewWithOptions(opts).Warnings(); len(w) != 4 || w[3].Message != marker {
		t.Errorf("Warnings() = %+v, want 3 warnings and the drop marker", w)
	}
	var got []ignore.ParseWarning
	opts.WarningHandler = func(w ignore.ParseWarning) { got = append(got, w) }
	ignore.NewWithOptions(opts)
	if len(got) != 4 || got[3].Message != marker {
		t.Errorf("handler got %+v, want 3 warnings and the drop marker", got)
	}
}
