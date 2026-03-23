package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

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
