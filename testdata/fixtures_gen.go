//go:build ignore

// This program generates test fixture files with specific encodings.
// Run with: go run fixtures_gen.go
//
// It creates:
//   - crlf.gitignore: Windows line endings (CRLF)
//   - with-bom.gitignore: UTF-8 BOM prefix
//   - pathological.gitignore: Patterns that stress backtracking
//   - large.gitignore: 100+ rules for benchmarking

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	generators := []struct {
		name string
		fn   func() []byte
	}{
		{"crlf.gitignore", generateCRLF},
		{"with-bom.gitignore", generateWithBOM},
		{"pathological.gitignore", generatePathological},
		{"realistic/large.gitignore", generateLarge},
	}

	for _, g := range generators {
		path := filepath.Join(dir, g.name)

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory for %s: %v\n", path, err)
			continue
		}

		content := g.fn()
		if err := os.WriteFile(path, content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			continue
		}
		fmt.Printf("Generated: %s (%d bytes)\n", path, len(content))
	}
}

// generateCRLF creates a gitignore with Windows CRLF line endings
func generateCRLF() []byte {
	lines := []string{
		"# Windows line endings test",
		"*.log",
		"build/",
		"!important.log",
		"",
		"# Nested patterns",
		"**/temp",
		"src/**/test",
	}

	var content []byte
	for _, line := range lines {
		content = append(content, []byte(line)...)
		content = append(content, '\r', '\n') // CRLF
	}
	return content
}

// generateWithBOM creates a gitignore with UTF-8 BOM prefix
func generateWithBOM() []byte {
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := []byte(`# UTF-8 BOM test file
# The BOM (EF BB BF) should be stripped during parsing

*.log
*.tmp
build/
node_modules/

# Unicode patterns
日本語.txt
données/
`)
	return append(bom, content...)
}

// generatePathological creates patterns that stress the matching algorithm
func generatePathological() []byte {
	return []byte(`# Pathological patterns for stress testing backtracking

# Multiple double-stars
a/**/b/**/c
a/**/b/**/c/**/d
a/**/b/**/c/**/d/**/e

# Double-star with wildcards
**/*.log
**/test_*
**/*_test.go

# Deeply nested double-star
src/**/internal/**/generated/**

# Many alternates (would be worse with character classes)
*.log
*.tmp
*.bak
*.swp
*.swo
*~
*.orig
*.rej

# Complex combinations
**/node_modules/**/package.json
src/**/test/**/*_test.go
`)
}

// generateLarge creates 100+ rules for benchmark testing
func generateLarge() []byte {
	var content []byte
	content = append(content, []byte("# Large gitignore for benchmark testing\n\n")...)

	// Common patterns
	common := []string{
		"*.log", "*.tmp", "*.bak", "*.swp", "*.swo",
		"build/", "dist/", "out/", "target/",
		"node_modules/", "vendor/", ".venv/",
		".git/", ".svn/", ".hg/",
		".idea/", ".vscode/", "*.sublime-*",
		".DS_Store", "Thumbs.db", "desktop.ini",
		"*.pyc", "*.pyo", "__pycache__/",
		"*.class", "*.jar",
		"*.o", "*.a", "*.so", "*.dylib",
		"*.exe", "*.dll",
	}

	for _, p := range common {
		content = append(content, []byte(p+"\n")...)
	}

	content = append(content, []byte("\n# Generated patterns\n")...)

	// Generate many patterns
	prefixes := []string{"", "src/", "lib/", "pkg/", "internal/", "test/"}
	extensions := []string{".log", ".tmp", ".cache", ".out", ".gen"}

	for i := 0; i < 20; i++ {
		for _, prefix := range prefixes {
			for _, ext := range extensions {
				pattern := fmt.Sprintf("%s*%s\n", prefix, ext)
				content = append(content, []byte(pattern)...)
			}
		}
	}

	// Add some double-star patterns
	content = append(content, []byte("\n# Double-star patterns\n")...)
	for i := 0; i < 10; i++ {
		content = append(content, []byte(fmt.Sprintf("**/generated%d/\n", i))...)
		content = append(content, []byte(fmt.Sprintf("**/.cache%d/\n", i))...)
	}

	// Add some negations
	content = append(content, []byte("\n# Negations\n")...)
	content = append(content, []byte("!important.log\n")...)
	content = append(content, []byte("!.gitkeep\n")...)
	content = append(content, []byte("!build/release/\n")...)

	return content
}
