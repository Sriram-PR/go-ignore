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

	// Handle basePath scoping
	matchSegments := pathSegments

	if r.basePath != "" {
		// Path must be under basePath
		if !strings.HasPrefix(path, r.basePath+"/") && path != r.basePath {
			return false
		}
		// Remove basePath prefix for matching
		if path == r.basePath {
			matchSegments = []string{}
		} else {
			matchPath := path[len(r.basePath)+1:] // +1 for the /
			matchSegments = splitPath(matchPath)
		}
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
		// Anchored: must match from the start
		if prefixMatch {
			return matchSegmentsPrefix(r.segments, matchSegments, ctx, caseInsensitive)
		}
		return matchSegmentsExact(r.segments, matchSegments, ctx, caseInsensitive)
	}

	// Floating: can match at any position in path
	// Try matching starting from each position
	maxStart := len(matchSegments) - len(r.segments)
	if prefixMatch {
		// For prefix matching, we can start later since we don't need exact length
		maxStart = len(matchSegments) - 1
	}
	for i := 0; i <= maxStart; i++ {
		if !ctx.tick() {
			return false // Limit exceeded
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
	// e.g., pattern "**/foo" with 1 segment can match path "foo" with 1 segment
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
// Supports * as "match zero or more characters" and ? as "match exactly one character".
// Backtracking is bounded by the shared matchContext.
func matchGlob(pattern, s string, ctx *matchContext) bool {
	hasWild := strings.ContainsAny(pattern, "*?\\")

	// Fast path: no wildcards or escapes
	if !hasWild {
		return pattern == s
	}

	// Fast path: single * matches everything
	if pattern == "*" {
		return true
	}

	// Fast paths only apply when there are no ? wildcards and no \ escapes
	hasEscape := strings.Contains(pattern, "\\")
	hasQuestion := strings.Contains(pattern, "?")
	if !hasQuestion && !hasEscape {
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

		if pattern[0] == '?' {
			// ? matches exactly one character
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
			continue
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
