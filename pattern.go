package ignore

import (
	"strings"
)

// ParseWarning represents a warning from parsing a .gitignore line.
// Warnings are generated for malformed patterns that are skipped during parsing.
type ParseWarning struct {
	Pattern string // The problematic pattern
	Message string // Human-readable warning message
	Line    int    // Line number (1-indexed)
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

	// Step 4: Handle escaped # at start (\# means literal #)
	// Note: We only support \# escape, not general escape sequences
	if strings.HasPrefix(line, "\\#") {
		line = line[1:] // Remove the backslash, keep the #
	}

	// Step 5: Check for negation
	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	}

	// Step 6: Check for directory-only (trailing /)
	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// Step 7: Normalize ./ prefix (Git treats ./foo as foo)
	line = strings.TrimPrefix(line, "./")

	// Step 8: Check if pattern is empty after stripping
	if line == "" {
		return nil, &ParseWarning{
			Line:    lineNum,
			Pattern: original,
			Message: "pattern is empty after processing",
		}
	}

	// Step 9: Determine anchoring
	// A pattern is anchored if:
	//   - It starts with / (explicit anchor)
	//   - It contains / anywhere except trailing (implicit anchor)
	// Exception: Patterns starting with **/ are NOT anchored (they float)
	anchored := false
	if strings.HasPrefix(line, "/") {
		anchored = true
		line = strings.TrimPrefix(line, "/")
		// Check again after removing /
		if line == "" {
			return nil, &ParseWarning{
				Line:    lineNum,
				Pattern: original,
				Message: "pattern is empty after removing leading slash",
			}
		}
	} else if strings.Contains(line, "/") {
		// Contains slash, but check for **/ prefix which makes it float
		if !strings.HasPrefix(line, "**/") {
			anchored = true
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
		} else if strings.Contains(part, "*") {
			seg.wildcard = true
		}

		segments = append(segments, seg)
	}

	return segments
}

// isDoubleStar checks if a segment is a ** pattern.
func (s *segment) isDoubleStar() bool {
	return s.doubleStar
}

// isWildcard checks if a segment contains wildcards (but is not **).
func (s *segment) isWildcard() bool {
	return s.wildcard && !s.doubleStar
}

// isLiteral checks if a segment is a plain literal (no wildcards).
func (s *segment) isLiteral() bool {
	return !s.wildcard && !s.doubleStar
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

// segmentsString returns a debug representation of segments.
func segmentsString(segs []segment) string {
	var parts []string
	for _, s := range segs {
		if s.doubleStar {
			parts = append(parts, "**")
		} else if s.wildcard {
			parts = append(parts, s.value+"(wild)")
		} else {
			parts = append(parts, s.value)
		}
	}
	return strings.Join(parts, "/")
}
