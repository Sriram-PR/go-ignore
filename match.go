package ignore

import (
	"strings"
)

// DefaultMaxBacktrackIterations is the default limit for pattern matching iterations.
// This prevents pathological patterns from causing excessive CPU usage.
// The budget is shared across all rules for a single Match call and covers both
// segment-level ** matching and character-level glob matching (*, ?).
// Can be overridden via MatcherOptions.
const DefaultMaxBacktrackIterations = 10000

// matchContext tracks state during matching to prevent runaway backtracking.
type matchContext struct {
	iterations int
	maxIter    int
}

// newMatchContext creates a new match context with the specified limit.
// If maxIter is 0, uses DefaultMaxBacktrackIterations.
// If maxIter is -1, no limit is applied (not recommended).
func newMatchContext(maxIter int) *matchContext {
	if maxIter == 0 {
		maxIter = DefaultMaxBacktrackIterations
	}
	return &matchContext{
		iterations: 0,
		maxIter:    maxIter,
	}
}

// tick increments the iteration counter and returns false if limit exceeded.
func (ctx *matchContext) tick() bool {
	ctx.iterations++
	if ctx.maxIter < 0 {
		return true // No limit
	}
	return ctx.iterations <= ctx.maxIter
}

// matchRule checks if a path matches a single rule.
// path should already be normalized.
// pathSegments is the path split by "/".
// isDir indicates whether the path is a directory.
// caseInsensitive enables case-insensitive matching.
// ctx is the shared backtrack budget for the entire Match call.
func matchRule(r *rule, path string, pathSegments []string, isDir bool, caseInsensitive bool, ctx *matchContext) bool {
	// Check if we've already exhausted the budget
	if !ctx.tick() {
		return false
	}

	matchSegments := resolveMatchSegments(r, path, pathSegments)
	if matchSegments == nil {
		return false // path not under basePath
	}

	// Empty path after basePath stripping
	if len(matchSegments) == 0 {
		// Only matches if pattern is also empty (shouldn't happen with valid rules)
		return len(r.segments) == 0
	}

	// Directory-only patterns:
	// - Match directories directly (isDir == true)
	// - Match files INSIDE matching directories (isDir == false, path is inside dir)
	// For the "inside dir" case, we use prefix matching
	prefixMatch := r.dirOnly && !isDir

	// Handle anchored vs floating patterns
	if r.anchored {
		if prefixMatch {
			return matchSegmentsPrefix(r.segments, matchSegments, ctx, caseInsensitive)
		}
		return matchSegmentsExact(r.segments, matchSegments, ctx, caseInsensitive)
	}

	return matchFloating(r, matchSegments, prefixMatch, caseInsensitive, ctx)
}

// resolveMatchSegments applies basePath scoping and returns the segments to match against.
// Returns nil if path is not under the rule's basePath.
func resolveMatchSegments(r *rule, path string, pathSegments []string) []string {
	if r.basePath == "" {
		return pathSegments
	}
	// Path must be under basePath
	if !strings.HasPrefix(path, r.basePath+"/") && path != r.basePath {
		return nil
	}
	if path == r.basePath {
		return []string{}
	}
	matchPath := path[len(r.basePath)+1:] // +1 for the /
	return splitPath(matchPath)
}

// matchFloating tries to match a floating (unanchored) pattern at any position in the path.
func matchFloating(r *rule, matchSegments []string, prefixMatch, caseInsensitive bool, ctx *matchContext) bool {
	maxStart := len(matchSegments) - len(r.segments)
	if prefixMatch {
		maxStart = len(matchSegments) - 1
	}
	for i := 0; i <= maxStart; i++ {
		if !ctx.tick() {
			return false
		}
		if prefixMatch {
			if matchSegmentsPrefix(r.segments, matchSegments[i:], ctx, caseInsensitive) {
				return true
			}
		} else {
			if matchSegmentsExact(r.segments, matchSegments[i:], ctx, caseInsensitive) {
				return true
			}
		}
	}

	// Special case: pattern with ** can match even if more segments than path
	if len(r.segments) > 0 && r.segments[0].doubleStar {
		if prefixMatch {
			return matchSegmentsPrefix(r.segments, matchSegments, ctx, caseInsensitive)
		}
		return matchSegmentsExact(r.segments, matchSegments, ctx, caseInsensitive)
	}

	return false
}

// matchSegmentsExact recursively matches pattern segments against path segments.
// This is the core matching algorithm with ** support.
func matchSegmentsExact(pattern []segment, path []string, ctx *matchContext, caseInsensitive bool) bool {
	// Check iteration limit
	if !ctx.tick() {
		return false
	}

	// Base cases
	if len(pattern) == 0 {
		return len(path) == 0
	}

	seg := pattern[0]

	// Handle ** (double-star)
	if seg.doubleStar {
		// ** can match zero or more path segments
		// Try matching remaining pattern against path starting at each position
		for i := 0; i <= len(path); i++ {
			if matchSegmentsExact(pattern[1:], path[i:], ctx, caseInsensitive) {
				return true
			}
			if !ctx.tick() {
				return false
			}
		}
		return false
	}

	// No more path segments but still have pattern segments (non-**)
	if len(path) == 0 {
		return false
	}

	// Match current segment
	if !matchSingleSegment(seg, path[0], caseInsensitive, ctx) {
		return false
	}

	// Recurse for remaining segments
	return matchSegmentsExact(pattern[1:], path[1:], ctx, caseInsensitive)
}

// matchSegmentsPrefix matches pattern as a PREFIX of path.
// Unlike matchSegmentsExact, this allows the path to have additional segments
// after the pattern is fully matched. Used for directory patterns matching
// files inside the directory.
func matchSegmentsPrefix(pattern []segment, path []string, ctx *matchContext, caseInsensitive bool) bool {
	// Check iteration limit
	if !ctx.tick() {
		return false
	}

	// Base case: pattern exhausted
	if len(pattern) == 0 {
		// For prefix matching, we need at least one more path segment
		// (the file must be INSIDE the directory, not the directory itself)
		return len(path) > 0
	}

	seg := pattern[0]

	// Handle ** (double-star)
	if seg.doubleStar {
		// ** can match zero or more path segments
		// Try matching remaining pattern against path starting at each position
		for i := 0; i <= len(path); i++ {
			if matchSegmentsPrefix(pattern[1:], path[i:], ctx, caseInsensitive) {
				return true
			}
			if !ctx.tick() {
				return false
			}
		}
		return false
	}

	// No more path segments but still have pattern segments (non-**)
	if len(path) == 0 {
		return false
	}

	// Match current segment
	if !matchSingleSegment(seg, path[0], caseInsensitive, ctx) {
		return false
	}

	// Recurse for remaining segments
	return matchSegmentsPrefix(pattern[1:], path[1:], ctx, caseInsensitive)
}

// matchSingleSegment matches a single pattern segment against a path segment.
// Handles literal strings, * wildcards, ? wildcards, and \ escapes.
// The matchContext is shared with the caller so glob-level backtracking
// counts against the same budget as segment-level matching.
func matchSingleSegment(seg segment, pathSeg string, caseInsensitive bool, ctx *matchContext) bool {
	if seg.doubleStar {
		// ** shouldn't reach here; handled in matchSegmentsExact
		return true
	}

	pattern := seg.value
	if caseInsensitive {
		// Pattern values are pre-lowercased at AddPatterns time,
		// so only the path segment needs lowering here.
		pathSeg = strings.ToLower(pathSeg)
	}

	if !seg.wildcard {
		// Literal match
		return pattern == pathSeg
	}

	// Wildcard matching (glob-style *, ?, \)
	return matchGlob(pattern, pathSeg, ctx)
}

// matchGlob matches a glob pattern against a string.
// Supports * as "match zero or more characters", ? as "match exactly one character",
// [...] as character classes, and \ as escape.
// Backtracking is bounded by the shared matchContext.
func matchGlob(pattern, s string, ctx *matchContext) bool {
	hasWild := strings.ContainsAny(pattern, "*?\\[")

	// Fast path: no wildcards or escapes
	if !hasWild {
		return pattern == s
	}

	// Fast path: single * matches everything
	if pattern == "*" {
		return true
	}

	// Fast paths only apply when there are no ?, \, or [ characters
	hasBracket := strings.Contains(pattern, "[")
	hasEscape := strings.Contains(pattern, "\\")
	hasQuestion := strings.Contains(pattern, "?")
	if !hasQuestion && !hasEscape && !hasBracket {
		// Fast path: prefix* pattern
		if strings.Count(pattern, "*") == 1 && strings.HasSuffix(pattern, "*") {
			prefix := pattern[:len(pattern)-1]
			return strings.HasPrefix(s, prefix)
		}

		// Fast path: *suffix pattern
		if strings.Count(pattern, "*") == 1 && strings.HasPrefix(pattern, "*") {
			suffix := pattern[1:]
			return strings.HasSuffix(s, suffix)
		}
	}

	// General case: use recursive matching
	return matchGlobRecursive(pattern, s, ctx)
}

// matchGlobRecursive performs recursive glob matching.
// This handles patterns with * (zero or more chars), ? (exactly one char),
// and \ (escape next character for literal matching).
// Backtracking is bounded by the shared matchContext to prevent pathological
// patterns (e.g., *a*a*a*a*b) from causing excessive CPU usage.
func matchGlobRecursive(pattern, s string, ctx *matchContext) bool {
	for len(pattern) > 0 {
		if !ctx.tick() {
			return false // Backtrack limit exceeded
		}

		if pattern[0] == '*' {
			return matchGlobStar(pattern, s, ctx)
		}

		if pattern[0] == '?' {
			// ? matches exactly one character
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
			continue
		}

		if pattern[0] == '[' {
			// Character class
			if len(s) == 0 {
				return false
			}
			// '/' never matches a character class (FNM_PATHNAME)
			if s[0] == '/' {
				return false
			}
			matched, newPos, valid := matchCharClass(pattern, 0, s[0])
			if valid {
				if !matched {
					return false
				}
				pattern = pattern[newPos:]
				s = s[1:]
				continue
			}
			// Invalid (unclosed bracket) — treat '[' as literal, fall through
		}

		if pattern[0] == '\\' && len(pattern) > 1 {
			// Backslash escapes the next character (literal match)
			pattern = pattern[1:] // skip the backslash
			// Fall through to literal character comparison below
		}

		// No more string to match
		if len(s) == 0 {
			return false
		}

		// Characters must match
		if pattern[0] != s[0] {
			return false
		}

		pattern = pattern[1:]
		s = s[1:]
	}

	return len(s) == 0
}

// matchGlobStar handles the * wildcard branch of glob matching.
// It skips consecutive stars, then tries matching the remaining pattern
// against increasingly longer consumed prefixes of s.
func matchGlobStar(pattern, s string, ctx *matchContext) bool {
	// Skip consecutive stars
	for len(pattern) > 0 && pattern[0] == '*' {
		pattern = pattern[1:]
	}
	// Trailing * matches rest of string
	if len(pattern) == 0 {
		return true
	}
	// Try matching * with increasing number of characters
	for i := 0; i <= len(s); i++ {
		if matchGlobRecursive(pattern, s[i:], ctx) {
			return true
		}
		if !ctx.tick() {
			return false
		}
	}
	return false
}

// matchCharClass checks if ch matches a character class starting at pattern[pos].
// pattern[pos] must be '['.
// Returns (matched, newPos, valid):
//   - matched: whether ch is in the class
//   - newPos: position after the closing ']'
//   - valid: whether the class was well-formed (has closing ']')
//
// If valid is false, the caller should treat '[' as a literal character.
func matchCharClass(pattern string, pos int, ch byte) (matched bool, newPos int, valid bool) {
	// pos points at '['
	i := pos + 1
	if i >= len(pattern) {
		return false, 0, false // unclosed
	}

	// Check for negation
	negate := false
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negate = true
		i++
	}

	// ']' as first member (or first after negation) is literal
	first := true
	inClass := false

	for i < len(pattern) {
		c := pattern[i]

		if c == ']' && !first {
			// End of class
			result := inClass
			if negate {
				result = !inClass
			}
			return result, i + 1, true
		}

		first = false

		// POSIX class like [:alpha:]
		if c == '[' && i+1 < len(pattern) && pattern[i+1] == ':' {
			matched, advance := matchCharClassPosix(pattern, i, ch)
			if matched {
				inClass = true
			}
			i += advance
			continue
		}

		// Backslash escape inside class
		if c == '\\' && i+1 < len(pattern) {
			i++ // skip backslash
			c = pattern[i]
			matched, advance := matchCharClassRange(pattern, i, c, ch)
			if matched {
				inClass = true
			}
			i += advance
			continue
		}

		// Check for range: a-z
		// '-' is literal if first, last, or adjacent to ']'
		if i+2 < len(pattern) && pattern[i+1] == '-' && pattern[i+2] != ']' {
			matched, advance := matchCharClassRange(pattern, i, c, ch)
			if matched {
				inClass = true
			}
			i += advance
			continue
		}

		// Literal character
		if ch == c {
			inClass = true
		}
		i++
	}

	// Reached end of pattern without ']' — unclosed bracket
	return false, 0, false
}

// matchCharClassPosix handles [:name:] POSIX class parsing inside a character class.
// Returns whether ch matched and how many bytes to advance past this element.
func matchCharClassPosix(pattern string, i int, ch byte) (matched bool, advance int) {
	end := strings.Index(pattern[i+2:], ":]")
	if end >= 0 {
		name := pattern[i+2 : i+2+end]
		pred := posixClass(name)
		if pred != nil {
			return pred(ch), 2 + end + 2 // skip past ":]"
		}
		// Invalid POSIX name: treat '[' as a literal member
		return ch == '[', 1
	}
	// No closing ":]" — treat '[' as literal member
	return ch == '[', 1
}

// matchCharClassRange handles range (a-z, \x-y) and literal matching inside a character class.
// lo is the already-resolved start character at pattern[i].
// Returns whether ch matched and how many bytes to advance.
func matchCharClassRange(pattern string, i int, lo byte, ch byte) (matched bool, advance int) {
	if i+2 < len(pattern) && pattern[i+1] == '-' && pattern[i+2] != ']' {
		hi := pattern[i+2]
		if hi == '\\' && i+3 < len(pattern) {
			hi = pattern[i+3]
			return lo <= hi && ch >= lo && ch <= hi, 4
		}
		return lo <= hi && ch >= lo && ch <= hi, 3
	}
	return ch == lo, 1
}

// posixClasses maps POSIX character class names to their predicates.
var posixClasses = map[string]func(byte) bool{
	"alpha": func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') },
	"digit": func(c byte) bool { return c >= '0' && c <= '9' },
	"alnum": func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') },
	"upper": func(c byte) bool { return c >= 'A' && c <= 'Z' },
	"lower": func(c byte) bool { return c >= 'a' && c <= 'z' },
	"space": func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' },
	"blank": func(c byte) bool { return c == ' ' || c == '\t' },
	"print": func(c byte) bool { return c >= 0x20 && c <= 0x7E },
	"graph": func(c byte) bool { return c > 0x20 && c <= 0x7E },
	"punct": func(c byte) bool {
		return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
	},
	"cntrl":  func(c byte) bool { return c < 0x20 || c == 0x7F },
	"xdigit": func(c byte) bool { return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') },
}

// posixClass returns a predicate for the named POSIX character class,
// or nil if the name is not recognized.
func posixClass(name string) func(byte) bool {
	return posixClasses[name]
}

// splitPath splits a normalized path into segments.
// Empty segments (from leading/trailing/double slashes) are filtered out.
func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := strings.Split(path, "/")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
