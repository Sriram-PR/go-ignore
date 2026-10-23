package ignore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// BenchmarkNew measures Matcher creation overhead
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

// BenchmarkAddPatterns_Small measures adding a small gitignore
func BenchmarkAddPatterns_Small(b *testing.B) {
	b.ReportAllocs()
	content := []byte("*.log\nbuild/\nnode_modules/\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.AddPatterns("", content)
	}
}

// BenchmarkAddPatterns_Medium measures adding a medium gitignore
func BenchmarkAddPatterns_Medium(b *testing.B) {
	b.ReportAllocs()
	content := []byte(`
# Dependencies
node_modules/
vendor/
.venv/

# Build
build/
dist/
*.exe
*.dll
*.so

# Logs
*.log
logs/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db

# Environment
.env
.env.*
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.AddPatterns("", content)
	}
}

// BenchmarkAddPatterns_Large measures adding a large gitignore
func BenchmarkAddPatterns_Large(b *testing.B) {
	b.ReportAllocs()
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "*.ext%d\n", i)
		fmt.Fprintf(&sb, "dir%d/\n", i)
		fmt.Fprintf(&sb, "**/cache%d/\n", i)
	}
	content := []byte(sb.String())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.AddPatterns("", content)
	}
}

// BenchmarkMatch_Miss measures matching a non-ignored path
func BenchmarkMatch_Miss(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
	}
}

// BenchmarkMatch_Hit measures matching an ignored path
func BenchmarkMatch_Hit(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("debug.log", false)
	}
}

// BenchmarkMatch_DirPattern measures matching inside ignored directory
func BenchmarkMatch_DirPattern(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("node_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("node_modules/lodash/index.js", false)
	}
}

// BenchmarkMatch_DeepPath measures matching with deep paths
func BenchmarkMatch_DeepPath(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n**/temp/\n"))
	path := "a/b/c/d/e/f/g/h/i/j/k/l/m/n/test.log"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_DeepPath20 measures matching with 20 segments (within 32-element stack buffer).
func BenchmarkMatch_DeepPath20(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n"))
	path := "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/test.log" // 20 segments
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_DeepPath32 measures matching at the stack buffer boundary (32 segments).
func BenchmarkMatch_DeepPath32(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n"))
	path := "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/aa/bb/cc/dd/ee/test.log" // 32 segments
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_DeepPathNoNegation measures a 100-segment path that no rule
// matches and no negation re-includes.
func BenchmarkMatch_DeepPathNoNegation(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nnode_modules/\n.idea/\n*.tmp\n/dist\n"))
	path := strings.Repeat("d/", 99) + "main.go"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_DoubleStar measures ** pattern performance
func BenchmarkMatch_DoubleStar(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("**/logs/**\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/app/logs/error.log", false)
	}
}

// BenchmarkMatch_DoubleStarDeep measures ** on very deep paths
func BenchmarkMatch_DoubleStarDeep(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("**/target\n"))

	// Create a 20-level deep path
	parts := make([]string, 0, 21)
	for i := 0; i < 20; i++ {
		parts = append(parts, fmt.Sprintf("dir%d", i))
	}
	parts = append(parts, "target")
	path := strings.Join(parts, "/")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_ManyRules measures matching against many rules
func BenchmarkMatch_ManyRules(b *testing.B) {
	b.ReportAllocs()
	m := New()
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "*.ext%d\n", i)
	}
	m.AddPatterns("", []byte(sb.String()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
	}
}

// BenchmarkMatch_ManyRulesHit measures hitting a late rule
func BenchmarkMatch_ManyRulesHit(b *testing.B) {
	b.ReportAllocs()
	m := New()
	var sb strings.Builder
	for i := 0; i < 199; i++ {
		fmt.Fprintf(&sb, "*.ext%d\n", i)
	}
	sb.WriteString("*.target\n")
	m.AddPatterns("", []byte(sb.String()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("test.target", false)
	}
}

// BenchmarkMatch_Negation measures negation pattern performance
func BenchmarkMatch_Negation(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n!important.log\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("important.log", false)
	}
}

// BenchmarkMatch_NestedGitignore measures with multiple gitignore files
func BenchmarkMatch_NestedGitignore(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n"))
	m.AddPatterns("src", []byte("*.tmp\n"))
	m.AddPatterns("src/lib", []byte("*.bak\n"))
	m.AddPatterns("src/lib/internal", []byte("*.cache\n"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/lib/internal/data.cache", false)
	}
}

// BenchmarkMatch_Pathological tests worst-case ** patterns
func BenchmarkMatch_Pathological(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("a/**/b/**/c/**/d\n"))

	// Path that requires lots of backtracking
	path := "a/x/x/x/x/x/b/x/x/x/x/c/x/x/x/x/d"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_PathologicalNoMatch tests backtracking with no match
func BenchmarkMatch_PathologicalNoMatch(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("a/**/b/**/c/**/d\n"))

	// Path that backtracks but doesn't match
	path := "a/x/x/x/x/x/b/x/x/x/x/c/x/x/x/x/e"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatchWithReason measures MatchWithReason overhead
func BenchmarkMatchWithReason(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.MatchWithReason("debug.log", false)
	}
}

// BenchmarkMatch_Concurrent measures concurrent access
func BenchmarkMatch_Concurrent(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\n**/node_modules/**\n"))

	b.RunParallel(func(pb *testing.PB) {
		paths := []string{"src/main.go", "debug.log", "build/out.js", "node_modules/x/y.js"}
		i := 0
		for pb.Next() {
			m.Match(paths[i%len(paths)], false)
			i++
		}
	})
}

// BenchmarkMatch_CaseInsensitive measures case-insensitive overhead
func BenchmarkMatch_CaseInsensitive(b *testing.B) {
	b.ReportAllocs()
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("", []byte("*.LOG\nBUILD/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("debug.log", false)
	}
}

// BenchmarkMatch_CaseInsensitiveUpper measures case-insensitive with uppercase input
func BenchmarkMatch_CaseInsensitiveUpper(b *testing.B) {
	b.ReportAllocs()
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("", []byte("*.log\nBUILD/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("Some/Path/To/DEBUG.LOG", false)
	}
}

// BenchmarkNormalizePath measures path normalization overhead
func BenchmarkNormalizePath(b *testing.B) {
	b.ReportAllocs()
	paths := []string{
		"src/main.go",
		"src\\lib\\file.go",
		"./src/main.go",
		"src//lib//file.go",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizePath(paths[i%len(paths)])
	}
}

// BenchmarkParseLines measures parsing overhead
func BenchmarkParseLines(b *testing.B) {
	b.ReportAllocs()
	content := []byte(`
*.log
*.tmp
build/
!important.log
**/cache/
src/**/test/
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseLines("", content, -1, -1, "")
	}
}

// BenchmarkMatchGlob measures glob matching
func BenchmarkMatchGlob(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("*.log", "test.log")
		}
	})
	b.Run("prefix", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("test_*", "test_foo_bar")
		}
	})
	b.Run("complex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("*test*spec*", "my_test_file_spec_v2")
		}
	})
}

// BenchmarkMatchGlob_CharClass measures character class matching
func BenchmarkMatchGlob_CharClass(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("[abc]", "b")
		}
	})
	b.Run("range", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("[a-z]", "m")
		}
	})
	b.Run("negated", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("[!0-9]", "a")
		}
	})
	b.Run("combined", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchGlob("*.[ch]", "main.c")
		}
	})
}

// BenchmarkWalkDir_NestedGitignores walks sibling directories that each have
// a .gitignore. ns/entry should not grow with the directory count. It uses
// the OS filesystem: fstest.MapFS scans its whole map on every Open, so its
// own cost per entry grows with the tree.
func BenchmarkWalkDir_NestedGitignores(b *testing.B) {
	for _, dirs := range []int{500, 2000} {
		b.Run(fmt.Sprintf("dirs=%d", dirs), func(b *testing.B) {
			root := b.TempDir()
			var rules strings.Builder
			for r := range 10 {
				fmt.Fprintf(&rules, "*.tmp%d\n", r)
			}
			for d := range dirs {
				dir := filepath.Join(root, fmt.Sprintf("d%04d", d))
				if err := os.Mkdir(dir, 0o755); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(rules.String()), 0o644); err != nil {
					b.Fatal(err)
				}
				for f := range 5 {
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.go", f)), nil, 0o644); err != nil {
						b.Fatal(err)
					}
				}
			}
			m := New()
			entries := 0
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entries = 0
				_ = m.WalkDir(root, func(string, fs.DirEntry, error) error {
					entries++
					return nil
				})
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*entries), "ns/entry")
		})
	}
}

// siblingTreeRoot and siblingTreeDir are the .gitignore files of a tree with
// 200 sibling directories: a Tree evaluates the root's rules and one
// directory's, a preloaded matcher every directory's.
const (
	siblingTreeRoot = "*.log\nbuild/\n.cache/\n*.o\n!keep.o\n"
	siblingTreeDir  = "*.tmp\ncache/\n!keep.tmp\n/dist\n*.bak\n"
)

func siblingTree() fstest.MapFS {
	fsys := fstest.MapFS{".gitignore": {Data: []byte(siblingTreeRoot)}}
	for i := range 200 {
		fsys[fmt.Sprintf("d%d/.gitignore", i)] = &fstest.MapFile{Data: []byte(siblingTreeDir)}
	}
	return fsys
}

func BenchmarkTree_CheckIsDirWarm(b *testing.B) {
	tr := New().TreeFS(siblingTree(), ".")
	_, _ = tr.CheckIsDir("d150/src/main.go", false)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = tr.CheckIsDir("d150/src/main.go", false)
	}
}

func BenchmarkTree_PreloadedMatchWithReason(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte(siblingTreeRoot))
	for i := range 200 {
		m.AddPatterns(fmt.Sprintf("d%d", i), []byte(siblingTreeDir))
	}
	b.ReportAllocs()
	for b.Loop() {
		m.MatchWithReason("d150/src/main.go", false)
	}
}

func BenchmarkTree_CheckCold(b *testing.B) {
	fsys := siblingTree()
	m := New()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = m.TreeFS(fsys, ".").Check("d150/src/main.go")
	}
}

// BenchmarkMatch_DeepPathFloatingRules measures the parent walk with many
// floating rules on a deep path (1000 rules, 1000 segments).
func BenchmarkMatch_DeepPathFloatingRules(b *testing.B) {
	var rules strings.Builder
	for range 1000 {
		rules.WriteString("x*y\n")
	}
	rules.WriteString("*.log\n!a.log\n")
	m := New()
	m.AddPatterns("", []byte(rules.String()))
	path := strings.Repeat("a/", 1000) + "a.log"
	b.ReportAllocs()
	for b.Loop() {
		m.Match(path, false)
	}
}
