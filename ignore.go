package ignore

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// MatchResult provides detailed information about a match decision.
//
// Read the public fields directly. Negated is exposed only as a method since
// it is a derived value (Matched && !Ignored) rather than stored state.
type MatchResult struct {
	// Rule is the pattern string of the last matching rule (empty if Matched == false).
	// If multiple rules matched, this is the final decisive rule.
	Rule string

	// Source identifies which file or stream supplied the matching rule. It is
	// the absolute path of the originating .gitignore when the matcher knows it
	// (set automatically by AddGlobalPatterns, AddSystemPatterns,
	// AddExcludePatterns, AddPatternsFromFile, LoadRepo, and the nested
	// .gitignore discovery done inside WalkDir). For rules added via
	// AddPatterns or AddPatternsReader — which carry only an in-memory blob
	// and no source path — Source is "". Empty if Matched == false.
	Source string

	// BasePath is the directory containing the .gitignore that had the matching rule.
	// Empty string means the root .gitignore.
	BasePath string

	// Line is the line number (1-indexed) in the .gitignore file.
	// Zero if Matched == false.
	Line int

	// Ignored indicates the final decision: true if the path should be ignored.
	// This accounts for negation rules.
	Ignored bool

	// Matched indicates whether any rule matched the path (regardless of negation).
	// If false, no rules matched and the path is not ignored (default behavior).
	// If true, at least one rule matched (including negation rules); check Ignored for the final result.
	Matched bool
}

// Negated reports whether the final matching rule was a negation rule (i.e.,
// the path was re-included). True iff Matched && !Ignored. The method form
// documents the derivation so callers do not have to compute it themselves.
func (r MatchResult) Negated() bool { return r.Matched && !r.Ignored }

// WarningHandler is called for each parse warning if set.
// The warning includes BasePath; no separate basePath argument is provided.
type WarningHandler func(warning ParseWarning)

// Default resource limits for pattern parsing.
const (
	// DefaultMaxPatterns is the maximum number of rules a Matcher will hold.
	// Excess rules are dropped and a ParseWarning is emitted for each batch
	// that is truncated or skipped.
	// Set MaxPatterns to 0 to use this default, or -1 for unlimited.
	DefaultMaxPatterns = 100_000

	// DefaultMaxPatternLength is the maximum length of a single pattern line.
	// Lines exceeding this are skipped and a ParseWarning is emitted.
	// Set MaxPatternLength to 0 to use this default, or -1 for unlimited.
	DefaultMaxPatternLength = 4096
)

// MatcherOptions configures Matcher behavior.
type MatcherOptions struct {
	// WarningHandler is invoked for each parse warning produced by AddPatterns
	// (and helpers that call it). If nil, warnings are collected and made
	// available through Warnings(). The handler is fixed at construction time
	// and cannot be changed afterward; this prevents the ordering bug where a
	// handler set after AddPatterns would silently miss earlier warnings.
	//
	// The handler may be invoked concurrently from multiple goroutines if
	// AddPatterns is called concurrently; callers must make the handler safe
	// for concurrent use. Handlers may safely call back into the matcher
	// (including AddPatterns) — no library-side mutex is held during dispatch.
	WarningHandler WarningHandler

	// MaxBacktrackIterations limits ** pattern matching iterations.
	// Default: DefaultMaxBacktrackIterations (10000).
	// Set to 0 to use the default. Any negative value raises the limit to the
	// internal safety cap (10,000,000 iterations) — true unlimited is not
	// supported and the cap still applies even with -1.
	MaxBacktrackIterations int

	// CaseInsensitive enables case-insensitive matching.
	// Default: false (case-sensitive, matching Git's default behavior).
	// Note: This affects pattern matching only, not filesystem behavior.
	CaseInsensitive bool

	// MaxPatterns limits the total number of rules a Matcher can hold.
	// Default: DefaultMaxPatterns (100000). Set to -1 for unlimited.
	MaxPatterns int

	// MaxPatternLength limits the length of individual pattern lines.
	// Lines exceeding this limit are skipped with a parse warning.
	// Default: DefaultMaxPatternLength (4096). Set to -1 for unlimited.
	MaxPatternLength int
}

// Matcher holds compiled gitignore rules.
//
// Thread Safety: Matcher is safe for concurrent use. Concurrent calls to
// AddPatterns and Match are logically safe and will never cause data races
// or corruption. However, interleaving AddPatterns with many concurrent Match
// calls introduces lock contention and may reduce throughput. For best
// performance, batch all AddPatterns calls before starting concurrent Match
// operations.
type Matcher struct {
	mu       sync.RWMutex
	rules    []rule
	warnings []ParseWarning
	opts     MatcherOptions
}

// New creates an empty Matcher with default options.
func New() *Matcher {
	return &Matcher{
		opts: MatcherOptions{
			MaxBacktrackIterations: DefaultMaxBacktrackIterations,
			MaxPatterns:            DefaultMaxPatterns,
			MaxPatternLength:       DefaultMaxPatternLength,
		},
	}
}

// NewWithOptions creates a Matcher with custom options.
func NewWithOptions(opts MatcherOptions) *Matcher {
	if opts.MaxBacktrackIterations == 0 {
		opts.MaxBacktrackIterations = DefaultMaxBacktrackIterations
	}
	if opts.MaxPatterns == 0 {
		opts.MaxPatterns = DefaultMaxPatterns
	}
	if opts.MaxPatternLength == 0 {
		opts.MaxPatternLength = DefaultMaxPatternLength
	}
	return &Matcher{
		opts: opts,
	}
}

// AddPatterns parses gitignore content and adds rules.
// basePath is the directory containing the .gitignore (empty string for root).
//
// Input normalization (applied automatically):
//   - UTF-8 BOM is stripped if present
//   - CRLF and CR line endings are normalized to LF
//   - Trailing whitespace on each line is trimmed
//
// Both nil and empty content produce no rules. Nil content returns immediately
// without acquiring locks; empty content goes through parsing (which yields nothing).
//
// Parse warnings are delivered through the configured WarningHandler (set via
// MatcherOptions); if no handler is configured, warnings are appended to an
// internal buffer accessible via Warnings().
//
// Thread-safe: can be called concurrently with Match.
// Performance note: For best performance when loading many .gitignore files,
// batch AddPatterns calls before starting concurrent Match operations to
// reduce lock contention.
func (m *Matcher) AddPatterns(basePath string, content []byte) {
	m.addPatternsFromSource(basePath, content, "")
}

// AddPatternsWithSource is identical to AddPatterns except that the supplied
// source label is recorded on every rule parsed from content and surfaced via
// MatchResult.Source for any matching path. Use it when patterns originate
// from somewhere with a meaningful logical name that is not a file on disk
// (database row, embedded config, network response, etc.); AddPatternsFromFile
// already handles the on-disk case.
//
// source is informational only — it is not parsed, normalized, or validated.
// Empty source is permitted (equivalent to AddPatterns) but provides no
// provenance, so callers should prefer AddPatterns in that case for clarity.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsWithSource(basePath, source string, content []byte) {
	m.addPatternsFromSource(basePath, content, source)
}

// addPatternsFromSource is the internal worker behind AddPatterns and
// AddPatternsWithSource; helpers that know the originating file
// (AddGlobalPatterns, AddExcludePatterns, AddSystemPatterns,
// AddPatternsFromFile, LoadRepo, and the nested-gitignore discovery inside
// WalkDir) call this directly so MatchResult.Source can identify which file
// produced a rule.
func (m *Matcher) addPatternsFromSource(basePath string, content []byte, source string) {
	if content == nil {
		return
	}

	// Normalize basePath once for consistent rule scoping and warning reporting.
	normalizedBase := normalizePath(basePath)

	// Parse rules (this doesn't need the lock)
	newRules, parseWarnings := parseLines(normalizedBase, content, m.opts.MaxPatternLength, source)

	// Pre-lowercase pattern segment values for case-insensitive matching.
	// This avoids calling strings.ToLower on every match call.
	if m.opts.CaseInsensitive {
		for i := range newRules {
			for j := range newRules[i].segments {
				seg := &newRules[i].segments[j]
				if !seg.doubleStar {
					seg.value = strings.ToLower(seg.value)
				}
			}
		}
	}

	// Acquire write lock to add rules and capture handler ref
	m.mu.Lock()

	// Enforce max patterns limit
	if m.opts.MaxPatterns >= 0 {
		remaining := m.opts.MaxPatterns - len(m.rules)
		if remaining <= 0 {
			parseWarnings = append(parseWarnings, ParseWarning{
				Pattern:  "",
				Message:  "maximum pattern count reached, new patterns skipped",
				BasePath: normalizedBase,
			})
			newRules = nil
		} else if len(newRules) > remaining {
			parseWarnings = append(parseWarnings, ParseWarning{
				Pattern:  "",
				Message:  "maximum pattern count reached, excess patterns truncated",
				BasePath: normalizedBase,
			})
			newRules = newRules[:remaining]
		}
	}

	m.rules = append(m.rules, newRules...)
	handler := m.opts.WarningHandler
	if handler == nil {
		m.warnings = append(m.warnings, parseWarnings...)
	}
	m.mu.Unlock()

	// Dispatch warnings outside the main lock so handlers can safely call back
	// into the matcher (including AddPatterns itself). The handler may be
	// invoked concurrently from multiple goroutines; callers are responsible
	// for making it safe for concurrent use.
	if handler != nil {
		for _, w := range parseWarnings {
			handler(w)
		}
	}
}

// AddPatternsReader reads gitignore content from r and calls AddPatterns.
// It is equivalent to io.ReadAll followed by AddPatterns, but avoids forcing
// callers to buffer the entire file themselves.
//
// If reading r fails, no rules are added and the read error is returned wrapped.
// A successful read with empty content is treated the same as AddPatterns(basePath, nil).
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsReader(basePath string, r io.Reader) error {
	if r == nil {
		return nil
	}
	content, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading patterns: %w", err)
	}
	m.AddPatterns(basePath, content)
	return nil
}

// Warnings returns all collected parse warnings.
// Only populated if no WarningHandler was set.
func (m *Matcher) Warnings() []ParseWarning {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external mutation
	if len(m.warnings) == 0 {
		return nil
	}
	result := make([]ParseWarning, len(m.warnings))
	copy(result, m.warnings)
	return result
}

// Match returns true if the path should be ignored.
// path should be relative to repository root using forward slashes.
// On Windows, backslashes are automatically normalized to forward slashes.
// On Linux/macOS, backslashes are treated as literal filename characters
// (matching Git's behavior).
// isDir indicates whether the path is a directory.
// Thread-safe: can be called concurrently.
func (m *Matcher) Match(path string, isDir bool) bool {
	result := m.MatchWithReason(path, isDir)
	return result.Ignored
}

// MatchWithReason returns detailed information about why a path matches.
// Useful for debugging complex .gitignore setups.
// Thread-safe: can be called concurrently.
//
// Result interpretation:
//   - Matched == false: No rules matched; path is not ignored (default)
//   - Matched == true, Ignored == true: Path is ignored by Rule
//   - Matched == true, Ignored == false: Path was ignored but re-included by negation Rule
func (m *Matcher) MatchWithReason(path string, isDir bool) MatchResult {
	// Normalize path
	path = normalizePath(path)
	if path == "" {
		return MatchResult{Ignored: false, Matched: false}
	}

	// opts is fixed at construction (see Matcher.opts) and safe to read
	// without holding mu. Doing the case-insensitive lowering and the
	// backtrack-context setup outside the read lock keeps the critical
	// section as tight as possible.
	var segBuf [32]string
	pathSegments := splitPathBuf(path, segBuf[:0])

	// Pre-lowercase path and segments once for case-insensitive matching,
	// instead of lowering per-segment per-rule in matchSingleSegment.
	// Re-split after lowering so segments point into the lowered string (1 alloc vs N+1).
	if m.opts.CaseInsensitive {
		lowered := strings.ToLower(path)
		if lowered != path {
			path = lowered
			pathSegments = splitPathBuf(path, segBuf[:0])
		}
	}

	// Single shared backtrack budget for the entire Match call.
	// This prevents pathological patterns across many rules from causing
	// excessive CPU usage — previously each rule got a fresh budget.
	ctx := newMatchContext(m.opts.MaxBacktrackIterations)

	m.mu.RLock()

	result := evaluateRules(m.rules, path, pathSegments, isDir, &ctx)

	// Spec: a file cannot be re-included if a parent directory is excluded.
	// Only walk ancestors when negation tried to re-include the path —
	// otherwise the result is already correct and we'd waste budget.
	if result.Matched && !result.Ignored && len(pathSegments) > 1 {
		for i := 1; i < len(pathSegments); i++ {
			ancestor := strings.Join(pathSegments[:i], "/")
			ancRes := evaluateRules(m.rules, ancestor, pathSegments[:i], true, &ctx)
			if ancRes.Matched && ancRes.Ignored {
				m.mu.RUnlock()
				return ancRes
			}
		}
	}

	m.mu.RUnlock()
	return result
}

// evaluateRules runs all rules against a single path with last-match-wins semantics.
func evaluateRules(rules []rule, path string, pathSegments []string, isDir bool, ctx *matchContext) MatchResult {
	var result MatchResult
	for i := range rules {
		r := &rules[i]
		if matchRule(r, path, pathSegments, isDir, ctx) {
			result.Matched = true
			result.Rule = r.pattern
			result.Source = r.source
			result.BasePath = r.basePath
			result.Line = r.line
			result.Ignored = !r.negate
		}
	}
	return result
}

// RuleCount returns the number of rules currently loaded.
// Useful for debugging and testing.
func (m *Matcher) RuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rules)
}
