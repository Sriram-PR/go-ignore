package ignore

import (
	"strings"
	"sync"
)

// MatchResult provides detailed information about a match decision.
type MatchResult struct {
	// Rule is the pattern string of the last matching rule (empty if Matched == false).
	// If multiple rules matched, this is the final decisive rule.
	Rule string

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

	// Negated indicates whether the matching rule was a negation (started with !).
	// When Negated == true and Matched == true, the path was re-included.
	Negated bool
}

// WarningHandler is called for each parse warning if set.
type WarningHandler func(basePath string, warning ParseWarning)

// Default resource limits for pattern parsing.
const (
	// DefaultMaxPatterns is the maximum number of rules a Matcher will hold.
	// Excess rules are silently dropped with a warning.
	// Set MaxPatterns to 0 to use this default, or -1 for unlimited.
	DefaultMaxPatterns = 100_000

	// DefaultMaxPatternLength is the maximum length of a single pattern line.
	// Lines exceeding this are skipped with a warning.
	// Set MaxPatternLength to 0 to use this default, or -1 for unlimited.
	DefaultMaxPatternLength = 4096
)

// MatcherOptions configures Matcher behavior.
type MatcherOptions struct {
	// MaxBacktrackIterations limits ** pattern matching iterations.
	// Default: DefaultMaxBacktrackIterations (10000).
	// Set to 0 to use default. Set to -1 for unlimited (not recommended).
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
	handler  WarningHandler
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

// SetWarningHandler sets a callback for parse warnings.
// If set, warnings are reported via callback instead of being collected.
// Passing nil resets to collection mode (warnings available via Warnings()).
// IMPORTANT: Must be called before AddPatterns for the handler to receive warnings.
// If called after AddPatterns, only subsequent AddPatterns calls will use the handler.
func (m *Matcher) SetWarningHandler(fn WarningHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = fn
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
// Returns warnings for malformed patterns. Warnings are only returned if no
// WarningHandler was set via SetWarningHandler; otherwise warnings go to the handler.
//
// Thread-safe: can be called concurrently with Match.
// Performance note: For best performance when loading many .gitignore files,
// batch AddPatterns calls before starting concurrent Match operations to
// reduce lock contention.
func (m *Matcher) AddPatterns(basePath string, content []byte) []ParseWarning {
	if content == nil {
		return nil
	}

	// Normalize basePath once for consistent rule scoping and warning reporting.
	normalizedBase := normalizeBasePath(basePath)

	// Parse rules (this doesn't need the lock)
	newRules, parseWarnings := parseLines(normalizedBase, content, m.opts.MaxPatternLength)

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
	handler := m.handler
	if handler == nil {
		m.warnings = append(m.warnings, parseWarnings...)
	}
	m.mu.Unlock()

	// Dispatch warnings outside lock to prevent deadlock if handler calls back
	if handler != nil {
		for _, w := range parseWarnings {
			handler(normalizedBase, w)
		}
		return nil
	}
	return parseWarnings
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

	pathSegments := splitPath(path)

	m.mu.RLock()

	// Pre-lowercase path and segments once for case-insensitive matching,
	// instead of lowering per-segment per-rule in matchSingleSegment.
	if m.opts.CaseInsensitive {
		path = strings.ToLower(path)
		for i, seg := range pathSegments {
			pathSegments[i] = strings.ToLower(seg)
		}
	}

	var result MatchResult

	// Single shared backtrack budget for the entire Match call.
	// This prevents pathological patterns across many rules from causing
	// excessive CPU usage — previously each rule got a fresh budget.
	ctx := newMatchContext(m.opts.MaxBacktrackIterations)

	// Evaluate rules in order (last match wins)
	for i := range m.rules {
		r := &m.rules[i]

		if matchRule(r, path, pathSegments, isDir, &ctx) {
			result.Matched = true
			result.Rule = r.pattern
			result.BasePath = r.basePath
			result.Line = r.line
			result.Negated = r.negate

			if r.negate {
				result.Ignored = false
			} else {
				result.Ignored = true
			}
		}
	}

	m.mu.RUnlock()
	return result
}

// RuleCount returns the number of rules currently loaded.
// Useful for debugging and testing.
func (m *Matcher) RuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rules)
}
