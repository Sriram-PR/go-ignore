package ignore

import (
	"fmt"
	"io"
	"slices"
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
	// the path of the originating .gitignore when the matcher knows it, set
	// automatically by AddGlobalPatterns, AddSystemPatterns,
	// AddExcludePatterns, AddPatternsFromFile, LoadRepo, and the nested
	// .gitignore discovery inside WalkDir (an OS path, as given) and
	// WalkDirFS (an fs.FS path). For rules added via AddPatterns or
	// AddPatternsReader, which carry only an in-memory blob, Source is "".
	// Empty if Matched == false.
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

	// MaxBacktrackIterations was a per-Match budget for wildcard backtracking.
	// When it ran out, every later rule was skipped.
	//
	// Deprecated: matching no longer backtracks exponentially, so no input
	// can exhaust a budget and Match always gives git's answer. The value is
	// ignored.
	MaxBacktrackIterations int

	// CaseInsensitive enables case-insensitive matching. Only ASCII letters
	// are folded, as in git.
	// Default: false (case-sensitive, matching Git's default behavior).
	// Note: This affects pattern matching only, not filesystem behavior.
	CaseInsensitive bool

	// MaxPatterns limits the total number of rules a Matcher can hold.
	// Default: DefaultMaxPatterns (100000). Set to -1 for unlimited.
	//
	// Together with MaxPatternLength it bounds memory. A rule keeps about
	// 17 times its length when it has many short segments (a 4 KB a/a/...
	// pattern keeps about 70 KB), so the defaults allow about 7 GB. Lower
	// either limit for untrusted input.
	MaxPatterns int

	// MaxPatternLength limits the length of individual pattern lines.
	// Lines exceeding this limit are skipped with a parse warning.
	// Default: DefaultMaxPatternLength (4096). Set to -1 for unlimited.
	// See MaxPatterns for the memory these two limits allow.
	MaxPatternLength int

	// IgnoreFiles names extra per-directory ignore files, such as .ignore
	// or .aiignore, read in each directory right after its .gitignore, in
	// order, under the same rules, wherever .gitignore is read: the
	// walkers, Tree, and LoadRepo for the root. A later name beats an
	// earlier one in the same directory; a deeper directory's files beat a
	// shallower one's. Each must be a plain file name: no "/" or "\", not
	// "", ".", ".." or ".gitignore". A bad or repeated name is dropped with
	// a ParseWarning. NewWithOptions copies the list.
	IgnoreFiles []string

	// Overrides are gitignore pattern lines, scoped to the root, checked
	// after every other rule, nested ignore files a walker or Tree reads
	// included, so a match here decides. A "!" line re-includes, within
	// git's rule that the contents of an ignored directory stay ignored.
	// MatchResult.Source is "overrides" and Line is the 1-based index in
	// Overrides. An element containing "\n" or "\r" is dropped with a
	// ParseWarning. Override rules count toward MaxPatterns and are parsed
	// first. NewWithOptions reads them once; New and the zero value Matcher
	// have none.
	Overrides []string
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
	// mu guards rules and warnings. rules is append-only: a rule is never
	// changed once added, so a reader may take rules[:len:len] under the
	// read lock and use it after unlocking (match, Tree). The walker clones
	// instead, since it appends nested rules.
	mu       sync.RWMutex
	rules    []rule
	warnings []ParseWarning
	// overrides is parsed from opts.Overrides by NewWithOptions and never
	// changed after, so it is read without mu. Every reader evaluates it
	// after all other rules.
	overrides []rule
	// opts is set before the Matcher is shared and never changed after, so
	// it is read without mu.
	opts MatcherOptions
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
	m := &Matcher{opts: opts}
	m.opts.Overrides = nil // parsed into m.overrides
	names, nameWarnings := validIgnoreFiles(opts.IgnoreFiles)
	m.opts.IgnoreFiles = names
	overrides, overrideWarnings := m.parseOverrides(opts.Overrides)
	m.overrides = overrides
	m.dispatch(slices.Concat(nameWarnings, overrideWarnings))
	return m
}

// validIgnoreFiles returns a copy of names without those that are not a
// plain file name or repeat an earlier one, with a warning for each.
func validIgnoreFiles(names []string) ([]string, []ParseWarning) {
	var out []string
	var warnings []ParseWarning
	for _, n := range names {
		switch {
		case n == "" || n == "." || n == ".." || n == ".gitignore" || strings.ContainsAny(n, `/\`):
			warnings = append(warnings, ParseWarning{Pattern: n, Message: "IgnoreFiles name is not a plain file name other than .gitignore, skipped"})
		case slices.Contains(out, n):
			warnings = append(warnings, ParseWarning{Pattern: n, Message: "IgnoreFiles name repeated, skipped"})
		default:
			out = append(out, n)
		}
	}
	return out, warnings
}

// overridesSource is MatchResult.Source for a rule from Overrides.
const overridesSource = "overrides"

// parseOverrides parses lines as one root-level file whose line N is
// lines[N-1], so a rule's Line is its index + 1.
func (m *Matcher) parseOverrides(lines []string) ([]rule, []ParseWarning) {
	if len(lines) == 0 {
		return nil, nil
	}
	var warnings []ParseWarning
	kept := make([]string, len(lines))
	for i, l := range lines {
		if strings.ContainsAny(l, "\r\n") {
			warnings = append(warnings, ParseWarning{Pattern: l, Message: "override contains a line break, skipped", Line: i + 1})
			continue
		}
		kept[i] = l
	}
	content := strings.Join(kept, "\n")
	// parseLines drops one BOM at the start of a file; an override is a
	// line, whose BOM is pattern text.
	if strings.HasPrefix(content, "\ufeff") {
		content = "\ufeff" + content
	}
	rules, parsed, truncated := parseLines("", []byte(content), m.effectiveMaxPatternLength(), m.effectiveMaxPatterns(), overridesSource)
	warnings = append(warnings, parsed...)
	if truncated {
		warnings = append(warnings, ParseWarning{Message: "maximum pattern count reached, excess patterns truncated"})
	}
	if m.opts.CaseInsensitive {
		lowerRules(rules)
	}
	return rules, warnings
}

// dispatch delivers construction-time warnings: to the handler if set,
// otherwise to Warnings().
func (m *Matcher) dispatch(ws []ParseWarning) {
	if len(ws) == 0 {
		return
	}
	if h := m.opts.WarningHandler; h != nil {
		for _, w := range ws {
			h(w)
		}
		return
	}
	m.mu.Lock()
	m.retainWarnings(ws, "")
	m.mu.Unlock()
}

// lowerRules pre-lowers what case-insensitive matching compares, so Match
// does not lower per call. git folds ASCII only.
func lowerRules(rules []rule) {
	for i := range rules {
		// Only the scoping key is lowered; basePath keeps the caller's
		// spelling for MatchResult.BasePath.
		rules[i].basePathSlash = asciiLower(rules[i].basePathSlash)
		for j := range rules[i].segments {
			seg := &rules[i].segments[j]
			if !seg.doubleStar {
				seg.value = asciiLower(seg.value)
			}
		}
	}
}

// The three resource limits treat a zero value as "use the documented default".
// New/NewWithOptions bake the defaults into opts, but a Matcher built another
// way (notably the zero value) skips that; resolving them per use keeps such a
// Matcher usable instead of taking a 0 limit literally and dropping every rule.
// A negative value (unlimited) passes through.
func (m *Matcher) effectiveMaxPatterns() int {
	if m.opts.MaxPatterns == 0 {
		return DefaultMaxPatterns
	}
	return m.opts.MaxPatterns
}

func (m *Matcher) effectiveMaxPatternLength() int {
	if m.opts.MaxPatternLength == 0 {
		return DefaultMaxPatternLength
	}
	return m.opts.MaxPatternLength
}

// AddPatterns parses gitignore content and adds rules.
// basePath is the directory containing the .gitignore (empty string for root).
//
// Input normalization (applied automatically, as git does):
//   - One leading UTF-8 BOM is stripped
//   - A CR before LF or at the end of content is dropped; any other CR is
//     pattern text
//   - Unescaped trailing spaces on each line are trimmed; tabs are kept
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

	// Parse no more rules than the matcher can still hold. Parsing itself
	// runs unlocked; the capacity is checked again under the lock below,
	// since a concurrent AddPatterns may have used some of it.
	maxRules := -1
	if maxPatterns := m.effectiveMaxPatterns(); maxPatterns >= 0 {
		m.mu.RLock()
		maxRules = max(maxPatterns-len(m.rules)-len(m.overrides), 0)
		m.mu.RUnlock()
	}
	newRules, parseWarnings, truncated := parseLines(normalizedBase, content, m.effectiveMaxPatternLength(), maxRules, source)

	// Pre-lowercase pattern segment values for case-insensitive matching.
	// This avoids lowering on every match call. git folds ASCII only.
	if m.opts.CaseInsensitive {
		lowerRules(newRules)
	}

	m.mu.Lock()

	if maxPatterns := m.effectiveMaxPatterns(); maxPatterns >= 0 {
		remaining := maxPatterns - len(m.rules) - len(m.overrides)
		if remaining <= 0 {
			parseWarnings = append(parseWarnings, ParseWarning{
				Pattern:  "",
				Message:  "maximum pattern count reached, new patterns skipped",
				BasePath: normalizedBase,
			})
			newRules = nil
		} else if truncated || len(newRules) > remaining {
			parseWarnings = append(parseWarnings, ParseWarning{
				Pattern:  "",
				Message:  "maximum pattern count reached, excess patterns truncated",
				BasePath: normalizedBase,
			})
			newRules = newRules[:min(len(newRules), remaining)]
		}
	}

	m.rules = append(m.rules, newRules...)
	handler := m.opts.WarningHandler
	if handler == nil {
		m.retainWarnings(parseWarnings, normalizedBase)
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

// maxRetainedWarnings bounds the warnings a Matcher collects for Warnings.
const maxRetainedWarnings = 10_000

// retainWarnings appends ws to the collected warnings, up to
// maxRetainedWarnings and then one marker, and drops the rest. m.mu must be
// held.
func (m *Matcher) retainWarnings(ws []ParseWarning, basePath string) {
	for _, w := range ws {
		switch {
		case len(m.warnings) < maxRetainedWarnings:
			m.warnings = append(m.warnings, w)
		case len(m.warnings) == maxRetainedWarnings:
			m.warnings = append(m.warnings, ParseWarning{
				Message:  "warning limit reached, further warnings dropped",
				BasePath: basePath,
			})
			return
		default:
			return
		}
	}
}

// AddPatternsReader reads gitignore content from r and calls AddPatterns.
// It is equivalent to io.ReadAll followed by AddPatterns, with the 100 MB
// limit below, but avoids forcing callers to buffer the entire file
// themselves.
//
// If reading r fails, no rules are added and the read error is returned wrapped.
// Reading stops after 100 MB, git's limit for a pattern file: more adds no
// rules and returns an error wrapping ErrPatternFileSkipped.
// A successful read with empty content is treated the same as AddPatterns(basePath, nil).
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsReader(basePath string, r io.Reader) error {
	return m.AddPatternsReaderWithSource(basePath, "", r)
}

// AddPatternsReaderWithSource is identical to AddPatternsReader except that the
// supplied source label is recorded on every rule read from r and surfaced via
// MatchResult.Source. Use it when patterns stream from something with a
// meaningful logical name that is not a file on disk (an HTTP response, a
// database row, an embedded asset); AddPatternsFromFile already handles the
// on-disk case.
//
// source is informational only — it is not parsed, normalized, or validated.
// If reading r fails, no rules are added and the read error is returned wrapped.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsReaderWithSource(basePath, source string, r io.Reader) error {
	return m.addPatternsReader(basePath, source, r, maxPatternFileSize)
}

// addPatternsReader is AddPatternsReaderWithSource with the size limit as a
// parameter, for tests.
func (m *Matcher) addPatternsReader(basePath, source string, r io.Reader, limit int64) error {
	if r == nil {
		return nil
	}
	content, err := readLimited(r, limit)
	if err != nil {
		return fmt.Errorf("reading patterns: %w", err)
	}
	m.AddPatternsWithSource(basePath, source, content)
	return nil
}

// Warnings returns the collected parse warnings: the first 10,000, then one
// "warning limit reached, further warnings dropped" entry. Only populated if
// no WarningHandler was set; a handler receives every warning.
func (m *Matcher) Warnings() []ParseWarning {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Clone(m.warnings)
}

// Match returns true if the path should be ignored.
// path should be relative to repository root using forward slashes.
// On Windows, backslashes are automatically normalized to forward slashes.
// On Linux/macOS, backslashes are treated as literal filename characters
// (matching Git's behavior).
// isDir indicates whether the path is a directory, and must be accurate:
// directory-only patterns (those ending in "/") match a directory only when
// isDir is true. Passing false for a path that is really a directory can make
// that directory be reported as not ignored (git needs the same information).
//
// As in git, a path inside an ignored directory is ignored, and no negation
// can re-include it: Match checks each parent directory, shallowest first,
// before taking the path's own verdict. Cost therefore grows with path depth,
// one pass over the rules per parent directory.
// Thread-safe: can be called concurrently.
func (m *Matcher) Match(path string, isDir bool) bool {
	result := m.MatchWithReason(path, isDir)
	return result.Ignored
}

// MatchWithReason returns detailed information about why a path matches.
// Useful for debugging complex .gitignore setups.
// isDir must be accurate for the same reason as in Match.
// Thread-safe: can be called concurrently.
//
// Result interpretation:
//   - Matched == false: No rules matched; path is not ignored (default)
//   - Matched == true, Ignored == true: Path is ignored by Rule
//   - Matched == true, Ignored == false: Path was ignored but re-included by negation Rule
//
// When a parent directory is ignored, the result describes the rule that
// ignored the outermost such parent, as git check-ignore -v reports it.
func (m *Matcher) MatchWithReason(path string, isDir bool) MatchResult {
	return m.match(path, isDir, true)
}

// match implements MatchWithReason. The walker passes checkParents=false: it
// prunes every directory match reports as ignored, so no path it visits has
// an ignored parent.
func (m *Matcher) match(path string, isDir, checkParents bool) MatchResult {
	// opts needs no lock (see Matcher). Lowering happens here, before the
	// snapshot, so nothing slow runs under mu.
	var segBuf [32]string
	_, path, pathSegments, ok := preparePath(path, m.opts.CaseInsensitive, segBuf[:0])
	if !ok {
		return MatchResult{}
	}

	m.mu.RLock()
	rules := m.rules[:len(m.rules):len(m.rules)]
	m.mu.RUnlock()

	// As in git, an ignored parent directory decides: no negation can
	// re-include a path below it.
	if checkParents {
		if res, ok := parentIgnored(rules, m.overrides, path, pathSegments); ok {
			return res
		}
	}
	return evaluateWithOverrides(rules, m.overrides, path, pathSegments, isDir)
}

// parentIgnored reports the rule that ignores the shallowest parent directory
// of path, so the outermost ignored parent is reported, as git check-ignore
// -v does. It slices path at each '/' instead of joining segments, which keeps
// the walk O(N) and allocation-free on deep paths.
func parentIgnored(rules, overrides []rule, path string, segs []string) (MatchResult, bool) {
	if len(segs) < 2 {
		return MatchResult{}, false
	}
	// Skip a leading slash so each parent is a name, not a "/" prefix.
	start := 0
	if path[0] == '/' {
		start = 1
	}
	n := 0
	for j := start; j < len(path); j++ {
		if path[j] != '/' {
			continue
		}
		n++
		res := evaluateWithOverrides(rules, overrides, path[start:j], segs[:n], true)
		if res.Matched && res.Ignored {
			return res, true
		}
	}
	return MatchResult{}, false
}

// preparePath normalizes path and splits it into segments, appending to buf.
// ok is false for a path that matches nothing: empty, resolving outside the
// root, or deeper than MaxPathDepth. Under icase, match and segs are lowered
// once here instead of per segment per rule; disk keeps the caller's
// spelling, for reading files.
func preparePath(path string, icase bool, buf []string) (disk, match string, segs []string, ok bool) {
	path = normalizePath(path)
	if path == "" {
		return "", "", nil, false
	}
	segs = splitPathBuf(path, buf)
	// The parent check runs every rule once per parent directory, so without
	// this cap a fuzzer or malicious caller could make one call
	// pathologically slow. See MaxPathDepth's docs.
	if len(segs) > MaxPathDepth {
		return "", "", nil, false
	}
	disk, match = path, path
	if icase {
		if lowered := asciiLower(path); lowered != path {
			match = lowered
			segs = splitPathBuf(match, buf[:0])
		}
	}
	return disk, match, segs, true
}

// evaluateRules runs all rules against a single path with last-match-wins semantics.
func evaluateRules(rules []rule, path string, pathSegments []string, isDir bool) MatchResult {
	var result MatchResult
	for i := range rules {
		r := &rules[i]
		if matchRule(r, path, pathSegments, isDir) {
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

// evaluateWithOverrides is evaluateRules over rules and then overrides, last
// match winning across both.
func evaluateWithOverrides(rules, overrides []rule, path string, segs []string, isDir bool) MatchResult {
	res := evaluateRules(rules, path, segs, isDir)
	if len(overrides) > 0 {
		if o := evaluateRules(overrides, path, segs, isDir); o.Matched {
			res = o
		}
	}
	return res
}

// RuleCount returns the number of rules currently loaded, Overrides
// included. Useful for debugging and testing.
func (m *Matcher) RuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rules) + len(m.overrides)
}

// asciiLower lowers A-Z only, as git's case folding does. Other bytes,
// including invalid UTF-8, are kept, so they stay distinct and ? still
// matches one byte.
func asciiLower(s string) string {
	i := 0
	for i < len(s) && (s[i] < 'A' || s[i] > 'Z') {
		i++
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(s[:i])
	for ; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}
