package ignore

import (
	"strings"
)

// DefaultMaxBacktrackIterations is the default limit for ** pattern matching.
// This prevents pathological patterns from causing excessive CPU usage.
// Can be overridden via MatcherOptions.
//
// Note: This is a global limit, not scaled by pattern complexity. Patterns with
// multiple ** segments (e.g., a/**/b/**/c/**/d) may hit this limit on deep trees.
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
func matchRule(r *rule, path string, pathSegments []string, isDir bool, caseInsensitive bool, maxIter int) bool {
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
		ctx := newMatchContext(maxIter)
		if prefixMatch {
			return matchSegmentsPrefix(r.segments, matchSegments, ctx, caseInsensitive)
		}
		return matchSegments_(r.segments, matchSegments, ctx, caseInsensitive)
	}

	// Floating: can match at any position in path
	// Try matching starting from each position
	ctx := newMatchContext(maxIter)
	maxStart := len(matchSegments) - len(r.segments)
	if prefixMatch {
		// For prefix matching, we can start later since we don't need exact length
		maxStart = len(matchSegments) - 1
	}
	for i := 0; i <= maxStart; i++ {
		if !ctx.tick() {
			return false // Limit exceeded
		}
		subCtx := newMatchContext(maxIter - ctx.iterations)
		if prefixMatch {
			if matchSegmentsPrefix(r.segments, matchSegments[i:], subCtx, caseInsensitive) {
				return true
			}
		} else {
			if matchSegments_(r.segments, matchSegments[i:], subCtx, caseInsensitive) {
				return true
			}
		}
		ctx.iterations += subCtx.iterations
	}

	// Special case: pattern with ** can match even if more segments than path
	// e.g., pattern "**/foo" with 1 segment can match path "foo" with 1 segment
	if len(r.segments) > 0 && r.segments[0].doubleStar {
		ctx := newMatchContext(maxIter)
		if prefixMatch {
			return matchSegmentsPrefix(r.segments, matchSegments, ctx, caseInsensitive)
		}
		return matchSegments_(r.segments, matchSegments, ctx, caseInsensitive)
	}

	return false
}

// matchSegments_ recursively matches pattern segments against path segments.
// This is the core matching algorithm with ** support.
func matchSegments_(pattern []segment, path []string, ctx *matchContext, caseInsensitive bool) bool {
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
			if matchSegments_(pattern[1:], path[i:], ctx, caseInsensitive) {
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
	if !matchSingleSegment(seg, path[0], caseInsensitive) {
		return false
	}

	// Recurse for remaining segments
	return matchSegments_(pattern[1:], path[1:], ctx, caseInsensitive)
}

// matchSegmentsPrefix matches pattern as a PREFIX of path.
// Unlike matchSegments_, this allows the path to have additional segments
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
	if !matchSingleSegment(seg, path[0], caseInsensitive) {
		return false
	}

	// Recurse for remaining segments
	return matchSegmentsPrefix(pattern[1:], path[1:], ctx, caseInsensitive)
}

// matchSingleSegment matches a single pattern segment against a path segment.
// Handles literal strings and * wildcards.
func matchSingleSegment(seg segment, pathSeg string, caseInsensitive bool) bool {
	if seg.doubleStar {
		// ** shouldn't reach here; handled in matchSegments_
		return true
	}

	pattern := seg.value
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		pathSeg = strings.ToLower(pathSeg)
	}

	if !seg.wildcard {
		// Literal match
		return pattern == pathSeg
	}

	// Wildcard matching (glob-style *)
	return matchGlob(pattern, pathSeg)
}

// matchGlob matches a glob pattern against a string.
// Supports * as "match zero or more characters".
// Does not support ? or character classes.
func matchGlob(pattern, s string) bool {
	// Fast path: no wildcards
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}

	// Fast path: single * matches everything
	if pattern == "*" {
		return true
	}

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

	// General case: use recursive matching
	return matchGlobRecursive(pattern, s)
}

// matchGlobRecursive performs recursive glob matching.
// This handles patterns with multiple * wildcards like "foo*bar*baz".
func matchGlobRecursive(pattern, s string) bool {
	for len(pattern) > 0 {
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
				if matchGlobRecursive(pattern, s[i:]) {
					return true
				}
			}
			return false
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
