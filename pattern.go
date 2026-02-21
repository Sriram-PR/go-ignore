package ignore

import (
	"strings"
)

// ParseWarning represents a warning from parsing a .gitignore line.
// Warnings are generated for malformed patterns that are skipped during parsing.
type ParseWarning struct {
	Pattern  string // The problematic pattern
	Message  string // Human-readable warning message
	Line     int    // Line number (1-indexed)
	BasePath string // Directory containing the .gitignore (empty for root)
}

// rule represents a single parsed gitignore pattern.
// Rules are evaluated in order; later rules can override earlier ones.
type rule struct {
	pattern  string    // original pattern (for debugging/reporting)
	basePath string    // directory scope (empty = root)
	segments []segment // parsed pattern segments for matching
	line     int       // line number in source file (1-indexed)
	negate   bool      // true if pattern started with !
	dirOnly  bool      // true if pattern ended with /
	anchored bool      // true if pattern should match from basePath only
}

// segment represents one part of a pattern split by "/".
// Each segment can be a literal string, contain wildcards, or be a double-star.
type segment struct {
	value      string // literal or pattern text (empty for **)
	wildcard   bool   // contains * (but not **) - requires glob matching
	doubleStar bool   // is ** - matches zero or more directories
}

// parseLines parses gitignore content into rules.
// It normalizes content (BOM, line endings) and processes each line.
// Returns parsed rules and any warnings for malformed patterns.
func parseLines(basePath string, content []byte) ([]rule, []ParseWarning) {
	// Normalize content (BOM, CRLF)
	content = normalizeContent(content)
	basePath = normalizeBasePath(basePath)

	lines := strings.Split(string(content), "\n")
	var rules []rule
	var warnings []ParseWarning

	for i, line := range lines {
		lineNum := i + 1 // 1-indexed

		r, warning := parseLine(line, lineNum, basePath)
		if warning != nil {
			warning.BasePath = basePath
			warnings = append(warnings, *warning)
		}
		if r != nil {
			rules = append(rules, *r)
		}
	}

	return rules, warnings
}

// parseLine parses a single line from a .gitignore file.
// Returns nil rule for empty lines, comments, and malformed patterns.
// Returns a warning for patterns that become empty after processing.
func parseLine(line string, lineNum int, basePath string) (*rule, *ParseWarning) {
	// Step 1: Trim trailing whitespace (Git behavior)
	line = trimTrailingWhitespace(line)

	// Step 2: Skip empty lines (no warning)
	if line == "" {
		return nil, nil
	}

	// Step 3: Skip comments
	if strings.HasPrefix(line, "#") {
		return nil, nil
	}

	// Store original for warning messages
	original := line

	// Step 4: Handle negation and \! escape
	// \! at start escapes the bang, treating it as literal (not negation).
	// Must check \! before ! to prevent misinterpreting escaped bangs.
	negate := false
	if strings.HasPrefix(line, "\\!") {
		line = line[1:] // Remove backslash, keep literal !
	} else if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	}

	// Step 5: Handle \# escape (after negation to support !\#foo)
	if strings.HasPrefix(line, "\\#") {
		line = line[1:] // Remove backslash, keep literal #
	}

	// Step 6: Check for directory-only (trailing /)
	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// Step 7: No ./ prefix stripping.
	// Git does NOT normalize ./ in patterns — ./foo matches nothing in git.
	// Users should not use ./ in patterns; if they do, it will be treated literally.

	// Step 8: Check if pattern is empty after stripping
	if line == "" {
		return nil, &ParseWarning{
			Line:    lineNum,
			Pattern: original,
			Message: "pattern is empty after processing",
		}
	}

	// Step 8b: Trailing backslash is an invalid pattern (per spec, never matches).
	// Count consecutive trailing backslashes: odd means a lone trailing \.
	if strings.HasSuffix(line, "\\") {
		bs := 0
		for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
			bs++
		}
		if bs%2 == 1 {
			return nil, &ParseWarning{
				Line:    lineNum,
				Pattern: original,
				Message: "trailing backslash is invalid (pattern never matches)",
			}
		}
	}

	// Step 9: Determine anchoring
	anchored, line, emptyAfterSlash := determineAnchoring(line)
	if emptyAfterSlash {
		return nil, &ParseWarning{
			Line:    lineNum,
			Pattern: original,
			Message: "pattern is empty after removing leading slash",
		}
	}

	// Step 10: Parse into segments
	segments := parseSegments(line)

	return &rule{
		pattern:  original,
		basePath: basePath,
		line:     lineNum,
		negate:   negate,
		dirOnly:  dirOnly,
		anchored: anchored,
		segments: segments,
	}, nil
}

// determineAnchoring resolves the anchoring state of a pattern line.
// A pattern is anchored if it starts with / or contains / (except **/ prefix).
// Returns the anchored flag, the trimmed line, and whether the line became empty
// after removing a leading slash.
func determineAnchoring(line string) (anchored bool, trimmed string, emptyAfterSlash bool) {
	if strings.HasPrefix(line, "/") {
		line = line[1:]
		if line == "" {
			return true, "", true
		}
		return true, line, false
	}
	if strings.Contains(line, "/") && !strings.HasPrefix(line, "**/") {
		return true, line, false
	}
	return false, line, false
}

// parseSegments splits a pattern by "/" and classifies each segment.
func parseSegments(pattern string) []segment {
	parts := strings.Split(pattern, "/")
	segments := make([]segment, 0, len(parts))

	for _, part := range parts {
		// Skip empty parts (from leading/trailing/double slashes)
		if part == "" {
			continue
		}

		seg := segment{value: part}

		if part == "**" {
			seg.doubleStar = true
			seg.value = ""
		} else if strings.Contains(part, "*") || strings.Contains(part, "?") || strings.Contains(part, "\\") || strings.Contains(part, "[") {
			// Segments with *, ?, or \ all require glob matching.
			// Backslash escapes (e.g., \* for literal *) are resolved during matching.
			seg.wildcard = true
		}

		segments = append(segments, seg)
	}

	return segments
}

// String returns a debug representation of a rule.
func (r *rule) String() string {
	var flags []string
	if r.negate {
		flags = append(flags, "negate")
	}
	if r.dirOnly {
		flags = append(flags, "dirOnly")
	}
	if r.anchored {
		flags = append(flags, "anchored")
	}

	flagStr := ""
	if len(flags) > 0 {
		flagStr = " [" + strings.Join(flags, ",") + "]"
	}

	baseStr := ""
	if r.basePath != "" {
		baseStr = " @" + r.basePath
	}

	return r.pattern + flagStr + baseStr
}
