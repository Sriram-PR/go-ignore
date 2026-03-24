package ignore_test

import (
	"fmt"

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

func ExampleMatcher_SetWarningHandler() {
	m := ignore.New()
	m.SetWarningHandler(func(basePath string, w ignore.ParseWarning) {
		fmt.Printf("line %d: %s\n", w.Line, w.Message)
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
