package ignore

import (
	"slices"
	"testing"
)

// The recursive, budgeted matcher from v1.3.0, kept as a reference for
// differential tests of the iterative matcher. Where it finishes within its
// budget and depth limit, its answer is correct.

const refMaxDepth = 200

type refBudget struct {
	iterations, maxIter, depth int
}

func newRefBudget() *refBudget { return &refBudget{maxIter: DefaultMaxBacktrackIterations} }

func (b *refBudget) tick() bool {
	b.iterations++
	return b.iterations <= b.maxIter
}

func (b *refBudget) exhausted() bool { return b.iterations >= b.maxIter }

// gaveUp reports whether the reference's answer may be wrong: its budget ran
// out, or it reached its depth limit.
func (b *refBudget) gaveUp(segs []segment) bool {
	return b.exhausted() || len(segs) >= refMaxDepth
}

func refMatchFloating(segs []segment, path []string, b *refBudget) bool {
	maxStart := len(path) - len(segs)
	start := 0
	if !slices.ContainsFunc(segs, isDoubleStar) {
		start = max(maxStart, 0)
	}
	for i := start; i <= maxStart; i++ {
		if b.exhausted() {
			return false
		}
		if refMatchSegmentsExact(segs, path[i:], b) {
			return true
		}
	}
	if len(segs) > 0 && segs[0].doubleStar {
		return refMatchSegmentsExact(segs, path, b)
	}
	return false
}

func refMatchSegmentsExact(pattern []segment, path []string, b *refBudget) bool {
	if b.exhausted() || b.depth >= refMaxDepth {
		return false
	}
	if len(pattern) == 0 {
		return len(path) == 0
	}
	seg := pattern[0]
	if seg.doubleStar {
		minI := 0
		if len(pattern) == 1 {
			minI = 1
		}
		b.depth++
		for i := minI; i <= len(path); i++ {
			if refMatchSegmentsExact(pattern[1:], path[i:], b) {
				b.depth--
				return true
			}
			if !b.tick() {
				b.depth--
				return false
			}
		}
		b.depth--
		return false
	}
	if len(path) == 0 {
		return false
	}
	if !seg.wildcard {
		if seg.value != path[0] {
			return false
		}
	} else if !refMatchGlob(seg.value, path[0], b) {
		return false
	}
	b.depth++
	result := refMatchSegmentsExact(pattern[1:], path[1:], b)
	b.depth--
	return result
}

func refMatchGlob(pattern, s string, b *refBudget) bool {
	for len(pattern) > 0 {
		if b.exhausted() {
			return false
		}
		if pattern[0] == '*' {
			return refMatchGlobStar(pattern, s, b)
		}
		if pattern[0] == '?' {
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
			continue
		}
		if pattern[0] == '[' {
			if len(s) == 0 {
				return false
			}
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
		}
		if pattern[0] == '\\' && len(pattern) > 1 {
			pattern = pattern[1:]
		}
		if len(s) == 0 {
			return false
		}
		if pattern[0] != s[0] {
			return false
		}
		pattern = pattern[1:]
		s = s[1:]
	}
	return len(s) == 0
}

func refMatchGlobStar(pattern, s string, b *refBudget) bool {
	for len(pattern) > 0 && pattern[0] == '*' {
		pattern = pattern[1:]
	}
	if len(pattern) == 0 {
		return true
	}
	for i := 0; i <= len(s); i++ {
		if refMatchGlob(pattern, s[i:], b) {
			return true
		}
		if !b.tick() {
			return false
		}
	}
	return false
}

// FuzzSegments compares the live segment matcher with the reference, for
// anchored and floating patterns.
func FuzzSegments(f *testing.F) {
	for _, s := range [][2]string{
		{"foo", "foo"},
		{"a/**/b", "a/x/y/b"},
		{"**/b", "b"},
		{"a/**", "a"},
		{"a/**", "a/b/c"},
		{"**/a/**/b/**/c", "x/a/y/b/z/c"},
		{"a/**/**/b", "a/b"},
		{"**", "x"},
		{"*/b", "a/b"},
		{"a/*a*a*b/c", "a/aaab/c"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pattern, path string) {
		segs := parseSegments(pattern)
		pathSegs := splitPath(path)
		for _, floating := range []bool{false, true} {
			// The parser only makes a pattern floating when it has one
			// segment or starts with **; the reference's start loop is
			// wrong for other shapes.
			if floating && len(segs) > 1 && !segs[0].doubleStar {
				continue
			}
			b := newRefBudget()
			var want bool
			if floating {
				want = refMatchFloating(segs, pathSegs, b)
			} else {
				want = refMatchSegmentsExact(segs, pathSegs, b)
			}
			got, ok := liveSegments(segs, pathSegs, floating)
			if !ok || b.gaveUp(segs) {
				continue
			}
			if got != want {
				t.Errorf("pattern %q path %q floating=%v: live %v, reference %v",
					pattern, path, floating, got, want)
			}
		}
	})
}

// liveSegments runs the production segment matcher. ok is false when its
// answer may be wrong.
func liveSegments(segs []segment, path []string, floating bool) (got, ok bool) {
	ctx := testCtx(DefaultMaxBacktrackIterations)
	if floating {
		got = matchFloating(&rule{segments: segs}, path, ctx)
	} else {
		got = matchSegmentsExact(segs, path, ctx)
	}
	return got, !ctx.exhausted() && len(segs) < maxRecursionDepth
}
