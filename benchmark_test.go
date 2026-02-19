package ignore

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkNew measures Matcher creation overhead
func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

// BenchmarkAddPatterns_Small measures adding a small gitignore
func BenchmarkAddPatterns_Small(b *testing.B) {
	content := []byte("*.log\nbuild/\nnode_modules/\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New()
		m.AddPatterns("", content)
	}
}

// BenchmarkAddPatterns_Medium measures adding a medium gitignore
func BenchmarkAddPatterns_Medium(b *testing.B) {
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
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf("*.ext%d\n", i))
		sb.WriteString(fmt.Sprintf("dir%d/\n", i))
		sb.WriteString(fmt.Sprintf("**/cache%d/\n", i))
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
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
	}
}

// BenchmarkMatch_Hit measures matching an ignored path
func BenchmarkMatch_Hit(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("debug.log", false)
	}
}

// BenchmarkMatch_DirPattern measures matching inside ignored directory
func BenchmarkMatch_DirPattern(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte("node_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("node_modules/lodash/index.js", false)
	}
}

// BenchmarkMatch_DeepPath measures matching with deep paths
func BenchmarkMatch_DeepPath(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte("*.log\n**/temp/\n"))
	path := "a/b/c/d/e/f/g/h/i/j/k/l/m/n/test.log"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(path, false)
	}
}

// BenchmarkMatch_DoubleStar measures ** pattern performance
func BenchmarkMatch_DoubleStar(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte("**/logs/**\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/app/logs/error.log", false)
	}
}

// BenchmarkMatch_DoubleStarDeep measures ** on very deep paths
func BenchmarkMatch_DoubleStarDeep(b *testing.B) {
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
	m := New()
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString(fmt.Sprintf("*.ext%d\n", i))
	}
	m.AddPatterns("", []byte(sb.String()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
	}
}

// BenchmarkMatch_ManyRulesHit measures hitting a late rule
func BenchmarkMatch_ManyRulesHit(b *testing.B) {
	m := New()
	var sb strings.Builder
	for i := 0; i < 199; i++ {
		sb.WriteString(fmt.Sprintf("*.ext%d\n", i))
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
	m := New()
	m.AddPatterns("", []byte("*.log\n!important.log\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("important.log", false)
	}
}

// BenchmarkMatch_NestedGitignore measures with multiple gitignore files
func BenchmarkMatch_NestedGitignore(b *testing.B) {
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
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.MatchWithReason("debug.log", false)
	}
}

// BenchmarkMatch_Concurrent measures concurrent access
func BenchmarkMatch_Concurrent(b *testing.B) {
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
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("", []byte("*.LOG\nBUILD/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("debug.log", false)
	}
}

// BenchmarkNormalizePath measures path normalization overhead
func BenchmarkNormalizePath(b *testing.B) {
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
		parseLines("", content)
	}
}

// BenchmarkMatchGlob measures glob matching
func BenchmarkMatchGlob(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchGlob("*.log", "test.log", newMatchContext(0))
		}
	})
	b.Run("prefix", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchGlob("test_*", "test_foo_bar", newMatchContext(0))
		}
	})
	b.Run("complex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchGlob("*test*spec*", "my_test_file_spec_v2", newMatchContext(0))
		}
	})
}
