package ignore

import (
	"slices"
	"strings"
)

// DefaultMaxBacktrackIterations was the default for
// MatcherOptions.MaxBacktrackIterations.
//
// Deprecated: matching no longer backtracks exponentially, so there is no
// budget to set. NewWithOptions still fills it in, and nothing reads it.
const DefaultMaxBacktrackIterations = 10000

// HardMaxBacktrackIterations was the cap applied when
// MatcherOptions.MaxBacktrackIterations is -1.
//
// Deprecated: matching no longer backtracks exponentially, so there is no
// budget to cap. Match costs at most O(P·N) segment comparisons per rule, for
// P pattern segments and N path segments.
const HardMaxBacktrackIterations = 10_000_000

// MaxPathDepth is the hard upper bound on the segment count of a path
// supplied to Match / MatchWithReason or to a Tree. Paths exceeding this depth
// short-circuit to "no match" without evaluating any rules. The cap exists
// because Match checks every parent directory against every rule, so its cost
// grows with N segments times M rules. Arbitrarily deep inputs (constructible
// by fuzzers or malicious callers) would otherwise be a denial-of-service
// vector.
//
// 4096 was chosen as ~the byte limit of PATH_MAX on Linux, vastly more than
// any realistic filesystem depth (typical: <30, deeply nested: <100). If
// you have a legitimate use case for matching beyond this depth, please
// open an issue describing it so we can consider exposing the cap as a
// MatcherOptions field.
const MaxPathDepth = 4096

// matchRule checks if a path matches a single rule.
// path should already be normalized (and pre-lowered if case-insensitive).
// pathSegments is the path split by "/".
// isDir indicates whether the path is a directory.
func matchRule(r *rule, path string, pathSegments []string, isDir bool) bool {
	matchSegs := resolveMatchSegments(r, path, pathSegments)
	if matchSegs == nil {
		return false // path not under basePath
	}

	// Empty path after basePath stripping
	if len(matchSegs) == 0 {
		// Only matches if pattern is also empty (shouldn't happen with valid rules)
		return len(r.segments) == 0
	}

	// A dir-only rule names directories only. Files inside a matching
	// directory are covered by Match's parent-directory check, as in git.
	if r.dirOnly && !isDir {
		return false
	}

	if r.anchored {
		return matchSegments(r.segments, matchSegs)
	}

	return matchFloating(r, matchSegs)
}

// resolveMatchSegments applies basePath scoping and returns the segments to match against.
// Returns nil if path is not under the rule's basePath.
func resolveMatchSegments(r *rule, path string, pathSegments []string) []string {
	if r.basePath == "" {
		return pathSegments
	}
	// Compare against basePathSlash, not basePath: under CaseInsensitive only
	// basePathSlash is lowercased to match the lowercased path.
	if path == r.basePathSlash[:len(r.basePathSlash)-1] {
		return []string{}
	}
	if !strings.HasPrefix(path, r.basePathSlash) {
		return nil
	}
	// Sub-slice the already-split path segments instead of re-splitting
	return pathSegments[r.baseSegCount:]
}

// matchFloating tries to match a floating (unanchored) pattern at any position in the path.
func matchFloating(r *rule, path []string) bool {
	// Most rules are one segment, such as *.log: compare the last segment.
	if len(r.segments) == 1 && !r.segments[0].doubleStar {
		return len(path) > 0 && matchSingleSegment(r.segments[0], path[len(path)-1])
	}
	// A floating pattern with ** always starts with it (see
	// determineAnchoring), and a leading ** already lets the match start at
	// any segment.
	if slices.ContainsFunc(r.segments, isDoubleStar) {
		return matchSegments(r.segments, path)
	}
	// Without **, a match consumes one path segment per pattern segment, so
	// it can only start at len(path) - len(r.segments).
	start := len(path) - len(r.segments)
	return start >= 0 && matchSegments(r.segments, path[start:])
}

func isDoubleStar(s segment) bool { return s.doubleStar }

// matchSegments matches pattern against all of path. ** matches any run of
// segments, except that a trailing ** needs at least one (abc/** matches
// abc/x, not abc, as in git), and so does a ** followed by \/; every other
// pattern segment matches exactly one. Like matchGlobLoop, on a mismatch it
// backs up only to the most recent **, so the cost is at most
// O(len(pattern)·len(path)) segment comparisons.
func matchSegments(pattern []segment, path []string) bool {
	p, i := 0, 0
	starP, starI := -1, 0
	for i < len(path) {
		if p < len(pattern) && pattern[p].doubleStar {
			need := 0 // segments the run must consume: one per **\/
			for p < len(pattern) && pattern[p].doubleStar {
				if pattern[p].minOne {
					need++
				}
				p++
			}
			// A trailing ** takes the rest, which must be one segment more
			// than the run needs.
			if p == len(pattern) {
				return len(path)-i > need
			}
			i += need
			starP, starI = p, i
			continue
		}
		if p < len(pattern) && matchSingleSegment(pattern[p], path[i]) {
			p, i = p+1, i+1
			continue
		}
		if starP < 0 {
			return false
		}
		starI++
		p, i = starP, starI
	}
	// Leftover ** here would have to match zero segments, which a trailing
	// ** may not.
	return p == len(pattern)
}

// matchSingleSegment matches a single pattern segment against a path segment.
// Handles literal strings, * wildcards, ? wildcards, \ escapes, and [ character classes.
func matchSingleSegment(seg segment, pathSeg string) bool {
	if seg.doubleStar {
		// ** must be handled by matchSegments;
		// if it ever leaks here, fail closed rather than report a spurious match.
		return false
	}

	pattern := seg.value
	// Note: case-insensitive lowering is done once in MatchWithReason,
	// not per-segment-per-rule here. Pattern values are pre-lowered at AddPatterns time.

	if !seg.wildcard {
		return pattern == pathSeg
	}

	return matchGlobSeg(&seg, pathSeg)
}

// matchGlobSeg matches a glob pattern against a string using pre-computed segment flags.
// This is the fast path used by matchSingleSegment.
func matchGlobSeg(seg *segment, s string) bool {
	pattern := seg.value

	// Fast path: single * matches everything
	if pattern == "*" {
		return true
	}

	// Fast paths only apply when there are no ?, \, or [ characters
	if !seg.hasQuestion && !seg.hasEscape && !seg.hasCharClass {
		// Fast path: prefix* pattern
		if seg.starCount == 1 && strings.HasSuffix(pattern, "*") {
			prefix := pattern[:len(pattern)-1]
			return strings.HasPrefix(s, prefix)
		}

		// Fast path: *suffix pattern
		if seg.starCount == 1 && strings.HasPrefix(pattern, "*") {
			suffix := pattern[1:]
			return strings.HasSuffix(s, suffix)
		}
	}

	return matchGlobLoop(pattern, s)
}

// matchGlobLoop matches pattern against s: * matches any run of bytes, and ?,
// a character class and \x each match exactly one. On a mismatch it backs up
// only to the most recent *, which suffices because every other token has a
// fixed width, so the cost is at most O(len(pattern)·len(s)).
func matchGlobLoop(pattern, s string) bool {
	p, i := 0, 0
	starP, starI := -1, 0
	for i < len(s) {
		if p < len(pattern) && pattern[p] == '*' {
			for p < len(pattern) && pattern[p] == '*' {
				p++
			}
			if p == len(pattern) {
				return true
			}
			starP, starI = p, i
			continue
		}
		if next, ok := matchGlobToken(pattern, p, s[i]); ok {
			p, i = next, i+1
			continue
		}
		if starP < 0 {
			return false
		}
		starI++
		p, i = starP, starI
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// matchGlobToken matches the one-byte token at pattern[p] (?, a character
// class, \x or a literal byte) against ch, and returns the position after it.
func matchGlobToken(pattern string, p int, ch byte) (next int, ok bool) {
	if p >= len(pattern) {
		return 0, false
	}
	switch pattern[p] {
	case '?':
		return p + 1, true
	case '[':
		// '/' never matches a character class (FNM_PATHNAME)
		if ch == '/' {
			return 0, false
		}
		if matched, newPos, valid := matchCharClass(pattern, p, ch); valid {
			return newPos, matched
		}
		// An unclosed [ is a literal. Parsing rejects such patterns, as git
		// never matches them, so only direct callers get here.
	case '\\':
		if p+1 < len(pattern) {
			p++
		}
	}
	return p + 1, pattern[p] == ch
}

// matchCharClass checks if ch matches a character class starting at pattern[pos].
// pattern[pos] must be '['.
// Returns (matched, newPos, valid):
//   - matched: whether ch is in the class
//   - newPos: position after the closing ']'
//   - valid: whether the class was well-formed (has closing ']')
//
// If valid is false, the caller treats '[' as a literal character. Parsed
// rules never hold an unclosed class (see classEnd).
func matchCharClass(pattern string, pos int, ch byte) (matched bool, newPos int, valid bool) {
	i := pos + 1
	if i >= len(pattern) {
		return false, 0, false // unclosed
	}

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
			i++
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

		if ch == c {
			inClass = true
		}
		i++
	}

	// Reached end of pattern without ']' — unclosed bracket
	return false, 0, false
}

// classEnd returns the index just past the bracket expression at
// pattern[pos], which must be '['. It follows the class scan in git's
// wildmatch.c, which does not stop at '/'. A non-empty problem is why git
// never matches a pattern holding this class.
func classEnd(pattern string, pos int) (end int, problem string) {
	i := pos + 1
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		i++
	}
	// rangeStart is git's prev_ch: whether the last member can start a range.
	// A range or a [:name:] can't.
	rangeStart := false
	for first := true; i < len(pattern); first = false {
		c := pattern[i]
		switch {
		case c == ']' && !first:
			return i + 1, ""
		case c == '\\':
			if i+1 >= len(pattern) {
				return 0, problemUnclosedClass
			}
			i += 2
			rangeStart = true
		case c == '-' && rangeStart && i+1 < len(pattern) && pattern[i+1] != ']':
			i++
			if pattern[i] == '\\' {
				if i+1 >= len(pattern) {
					return 0, problemUnclosedClass
				}
				i++
			}
			i++
			rangeStart = false
		case c == '[' && i+1 < len(pattern) && pattern[i+1] == ':':
			j := strings.IndexByte(pattern[i+2:], ']')
			if j < 0 {
				return 0, problemUnclosedClass
			}
			rb := i + 2 + j
			if j == 0 || pattern[rb-1] != ':' {
				// Not a [:name:]: '[' is a member, and the scan resumes at ':'.
				i++
				rangeStart = true
				break
			}
			if posixClass(pattern[i+2:rb-1]) == nil {
				return 0, problemUnknownClass
			}
			i = rb + 1
			rangeStart = false
		default:
			i++
			rangeStart = true
		}
	}
	return 0, problemUnclosedClass
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
		// Unknown name: parsing rejects it (see classEnd), so only direct
		// callers get here. Treat '[' as a literal member.
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
		advance = 3
		if hi == '\\' && i+3 < len(pattern) {
			hi = pattern[i+3]
			advance = 4
		}
		// git matches the start as a plain member, so a reversed range
		// still matches it.
		return ch == lo || (lo <= hi && ch >= lo && ch <= hi), advance
	}
	return ch == lo, 1
}

// posixClass returns a predicate for the named POSIX character class,
// or nil if the name is not recognized.
func posixClass(name string) func(byte) bool {
	switch name {
	case "alpha":
		return func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	case "digit":
		return func(c byte) bool { return c >= '0' && c <= '9' }
	case "alnum":
		return func(c byte) bool {
			return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		}
	case "upper":
		return func(c byte) bool { return c >= 'A' && c <= 'Z' }
	case "lower":
		return func(c byte) bool { return c >= 'a' && c <= 'z' }
	case "space":
		return func(c byte) bool {
			return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
		}
	case "blank":
		return func(c byte) bool { return c == ' ' || c == '\t' }
	case "print":
		return func(c byte) bool { return c >= 0x20 && c <= 0x7E }
	case "graph":
		return func(c byte) bool { return c > 0x20 && c <= 0x7E }
	case "punct":
		return func(c byte) bool {
			return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
		}
	case "cntrl":
		return func(c byte) bool { return c < 0x20 || c == 0x7F }
	case "xdigit":
		return func(c byte) bool {
			return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		}
	default:
		return nil
	}
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

// splitPathBuf splits a normalized path into segments using a caller-provided buffer.
// For paths with 32 or fewer segments (the common case), this avoids heap allocation.
// The buffer slice should be backed by a stack-allocated array: var buf [32]string; splitPathBuf(p, buf[:0]).
func splitPathBuf(path string, buf []string) []string {
	if path == "" {
		return buf[:0]
	}
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				buf = append(buf, path[start:i])
			}
			start = i + 1
		}
	}
	return buf
}
