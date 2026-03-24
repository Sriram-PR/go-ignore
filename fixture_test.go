package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFixtureMatchCorrectness loads fixture files and verifies match behavior.
func TestFixtureMatchCorrectness(t *testing.T) {
	type matchCase struct {
		path  string
		isDir bool
		want  bool
	}

	fixtures := []struct {
		path  string
		cases []matchCase
	}{
		{
			"testdata/simple.gitignore",
			[]matchCase{
				{"test.log", false, true},        // *.log
				{"build", true, true},            // build/
				{"build/output.js", false, true}, // inside build/
				{".DS_Store", false, true},       // .DS_Store
				{"src/main.go", false, false},    // no match
				{"node_modules", true, true},     // node_modules/
				{"Thumbs.db", false, true},       // Thumbs.db
			},
		},
		{
			"testdata/complex.gitignore",
			[]matchCase{
				{"important.log", false, false},        // negated by !important.log
				{"keep.log", false, false},             // negated by !keep.log
				{"debug.log", false, true},             // *.log
				{"src/generated/file.go", false, true}, // src/generated/
				{"docs/api/index.html", false, true},   // docs/**
				{"config.local", false, true},          // /config.local
				{"sub/config.local", false, false},     // anchored, not nested
				{"a/x/y/b", false, true},               // a/**/b
			},
		},
		{
			"testdata/edge-cases.gitignore",
			[]matchCase{
				{"foo bar.txt", false, true},     // spaces in filename
				{".env.local", false, true},      // .env.local
				{".env.production", false, true}, // .env.*
				{"#not-a-comment", false, true},  // \#not-a-comment
				{"test.tar.gz", false, true},     // *.tar.gz
				{"test.log.old", false, true},    // *.log.old
			},
		},
	}

	for _, f := range fixtures {
		t.Run(filepath.Base(f.path), func(t *testing.T) {
			content, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", f.path, err)
			}

			m := New()
			m.AddPatterns("", content)

			for _, tc := range f.cases {
				got := m.Match(tc.path, tc.isDir)
				if got != tc.want {
					t.Errorf("Match(%q, %v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
				}
			}
		})
	}
}

// TestLoadFixtures validates that all testdata fixture files parse without
// panics and produce the expected number of rules and no unexpected warnings.
func TestLoadFixtures(t *testing.T) {
	fixtures := []struct {
		path     string
		minRules int // minimum expected rules (0 means just check no panic)
	}{
		{"testdata/simple.gitignore", 7},
		{"testdata/complex.gitignore", 18},
		{"testdata/edge-cases.gitignore", 15},
		{"testdata/realistic/go.gitignore", 1},
		{"testdata/realistic/node.gitignore", 1},
		{"testdata/realistic/python.gitignore", 1},
		{"testdata/crlf.gitignore", 5},
		{"testdata/with-bom.gitignore", 6},
		{"testdata/pathological.gitignore", 16},
		{"testdata/realistic/large.gitignore", 100},
	}

	for _, f := range fixtures {
		t.Run(filepath.Base(f.path), func(t *testing.T) {
			content, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", f.path, err)
			}

			m := New()
			warnings := m.AddPatterns("", content)

			if m.RuleCount() < f.minRules {
				t.Errorf("fixture %s: got %d rules, want at least %d",
					f.path, m.RuleCount(), f.minRules)
			}

			// Log warnings for visibility but don't fail on them —
			// some fixtures intentionally contain edge cases that produce warnings.
			for _, w := range warnings {
				t.Logf("warning line %d: %s (%q)", w.Line, w.Message, w.Pattern)
			}
		})
	}
}
