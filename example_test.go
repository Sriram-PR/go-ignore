package ignore_test

import (
	"fmt"
	"os"
	"path/filepath"

	ignore "github.com/Sriram-PR/go-ignore"
)

func ExampleNew() {
	m := ignore.New()
	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))

	fmt.Println(m.Match("debug.log", false))
	fmt.Println(m.Match("src/main.go", false))
	fmt.Println(m.Match("important.log", false))
	fmt.Println(m.Match("build/output.js", false))
	// Output:
	// true
	// false
	// false
	// true
}

func ExampleMatcher_MatchWithReason() {
	m := ignore.New()
	m.AddPatterns("", []byte("*.log\n!important.log\n"))

	result := m.MatchWithReason("debug.log", false)
	fmt.Printf("ignored=%v rule=%q\n", result.Ignored, result.Rule)

	result = m.MatchWithReason("important.log", false)
	fmt.Printf("ignored=%v negated=%v rule=%q\n", result.Ignored, result.Negated, result.Rule)
	// Output:
	// ignored=true rule="*.log"
	// ignored=false negated=true rule="!important.log"
}

func ExampleMatcherOptions_warningHandler() {
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		WarningHandler: func(basePath string, w ignore.ParseWarning) {
			fmt.Printf("line %d: %s\n", w.Line, w.Message)
		},
	})
	m.AddPatterns("", []byte("*.log\n!\n"))
	// Output:
	// line 2: pattern is empty after processing
}

func ExampleMatcher_AddGlobalPatterns() {
	m := ignore.New()
	if err := m.AddGlobalPatterns(); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("loaded:", m.RuleCount() >= 0)
	// Output:
	// loaded: true
}

func ExampleMatcher_AddExcludePatterns() {
	// Simulate a repo with a .git/info/exclude file.
	gitDir, err := os.MkdirTemp("", "go-ignore-example-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(gitDir)

	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0o755); err != nil {
		fmt.Println("error:", err)
		return
	}
	excludeFile := filepath.Join(gitDir, "info", "exclude")
	if err := os.WriteFile(excludeFile, []byte("*.local\n"), 0o644); err != nil {
		fmt.Println("error:", err)
		return
	}

	m := ignore.New()
	if err := m.AddExcludePatterns(gitDir); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(m.Match("config.local", false))
	fmt.Println(m.Match("config.yaml", false))
	// Output:
	// true
	// false
}

func ExampleNewWithOptions() {
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		CaseInsensitive: true,
	})
	m.AddPatterns("", []byte("*.LOG\n"))

	fmt.Println(m.Match("debug.log", false))
	fmt.Println(m.Match("DEBUG.LOG", false))
	// Output:
	// true
	// true
}
