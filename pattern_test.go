package ignore

import (
	"testing"
)

func TestParseLine_Comments(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantNil bool
	}{
		{"simple comment", "# this is a comment", true},
		{"comment with spaces", "#    spaced comment", true},
		{"comment no space", "#comment", true},
		{"empty comment", "#", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if tt.wantNil && r != nil {
				t.Errorf("parseLine(%q) returned rule, want nil", tt.line)
			}
			if w != nil {
				t.Errorf("parseLine(%q) returned warning: %v", tt.line, w)
			}
		})
	}
}

func TestParseLine_EmptyLines(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty", ""},
		{"spaces only", "   "},
		{"tabs only", "\t\t"},
		{"mixed whitespace", " \t "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if r != nil {
				t.Errorf("parseLine(%q) returned rule, want nil", tt.line)
			}
			if w != nil {
				t.Errorf("parseLine(%q) returned warning, want nil (silent skip)", tt.line)
			}
		})
	}
}

func TestParseLine_Negation(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantNegate bool
		wantNil    bool
	}{
		{"negated pattern", "!important.log", true, false},
		{"not negated", "important.log", false, false},
		{"double negation", "!!double.log", true, false}, // First ! is negation
		{"negation only", "!", false, true},              // Empty after strip
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if tt.wantNil {
				if r != nil {
					t.Errorf("parseLine(%q) returned rule, want nil", tt.line)
				}
				if w == nil {
					t.Errorf("parseLine(%q) should have warning", tt.line)
				}
				return
			}
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil rule", tt.line)
			}
			if r.negate != tt.wantNegate {
				t.Errorf("parseLine(%q).negate = %v, want %v", tt.line, r.negate, tt.wantNegate)
			}
		})
	}
}

func TestParseLine_DirOnly(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantDirOnly bool
	}{
		{"directory pattern", "build/", true},
		{"file pattern", "build", false},
		{"nested directory", "src/build/", true},
		{"nested file", "src/build", false},
		{"negated directory", "!build/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.line, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.line)
			}
			if r.dirOnly != tt.wantDirOnly {
				t.Errorf("parseLine(%q).dirOnly = %v, want %v", tt.line, r.dirOnly, tt.wantDirOnly)
			}
		})
	}
}

func TestParseLine_Anchoring(t *testing.T) {
	// Based on Section 7 of the specification
	tests := []struct {
		name         string
		line         string
		wantAnchored bool
	}{
		// Not anchored: no / in pattern
		{"plain name", "foo", false},
		{"wildcard", "*.log", false},
		{"double wildcard", "**.log", false},

		// Anchored: starts with /
		{"leading slash", "/foo", true},
		{"leading slash nested", "/foo/bar", true},

		// Anchored: contains / (not at end)
		{"contains slash", "foo/bar", true},
		{"deep path", "foo/bar/baz", true},

		// Not anchored: trailing / only means directory
		{"trailing slash only", "foo/", false},

		// Not anchored: **/ prefix makes it float
		{"doublestar prefix", "**/foo", false},
		{"doublestar prefix nested", "**/foo/bar", false},
		{"doublestar middle", "a/**/b", true}, // Contains / other than **/

		// More edge cases
		{"doublestar only", "**", false},
		{"doublestar suffix", "foo/**", true}, // Contains / before **
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if w != nil {
				t.Errorf("parseLine(%q) warning: %v", tt.line, w)
			}
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.line)
			}
			if r.anchored != tt.wantAnchored {
				t.Errorf("parseLine(%q).anchored = %v, want %v", tt.line, r.anchored, tt.wantAnchored)
			}
		})
	}
}

func TestParseLine_EscapedHash(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantNil     bool
		wantPattern string
	}{
		{"escaped hash", "\\#not-a-comment", false, "\\#not-a-comment"},
		{"regular comment", "#comment", true, ""},
		{"escaped hash in middle", "foo\\#bar", false, "foo\\#bar"}, // Only leading \# is special
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.line, 1, "")
			if tt.wantNil {
				if r != nil {
					t.Errorf("parseLine(%q) returned rule, want nil", tt.line)
				}
				return
			}
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.line)
			}
			// The original pattern is stored
			if r.pattern != tt.wantPattern {
				t.Errorf("parseLine(%q).pattern = %q, want %q", tt.line, r.pattern, tt.wantPattern)
			}
		})
	}
}

func TestParseLine_DotSlashPrefix(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantAnchored bool
		wantSegments int
		wantDirOnly  bool
	}{
		{"dot slash prefix", "./foo", false, 1, false},       // ./ stripped, "foo" not anchored
		{"dot slash nested", "./foo/bar", true, 2, false},    // ./ stripped, "foo/bar" anchored by /
		{"no dot slash", "foo", false, 1, false},             // Same as ./foo
		{"dot slash with leading", "/./foo", true, 2, false}, // / makes it anchored

		// Edge cases - these produce valid (if unusual) patterns
		// "./" becomes "." after stripping trailing /, then ./ strip doesn't apply
		// "." is a valid pattern matching a file/directory literally named "."
		{"dot slash only", "./", false, 1, true}, // becomes "." (dirOnly)

		// "././foo" -> strip trailing / (none) -> strip ./ -> "./foo"
		// "./foo" contains "/" so it's anchored, segments: [".", "foo"]
		{"double dot slash", "././foo", true, 2, false}, // anchored because ./foo contains /
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if w != nil {
				t.Errorf("parseLine(%q) unexpected warning: %v", tt.line, w)
			}
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.line)
			}
			if r.anchored != tt.wantAnchored {
				t.Errorf("parseLine(%q).anchored = %v, want %v", tt.line, r.anchored, tt.wantAnchored)
			}
			if len(r.segments) != tt.wantSegments {
				t.Errorf("parseLine(%q) segments = %d, want %d (got: %v)",
					tt.line, len(r.segments), tt.wantSegments, segmentsString(r.segments))
			}
			if r.dirOnly != tt.wantDirOnly {
				t.Errorf("parseLine(%q).dirOnly = %v, want %v", tt.line, r.dirOnly, tt.wantDirOnly)
			}
		})
	}
}

func TestParseLine_TrailingWhitespace(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantSegValue string
	}{
		{"trailing space", "foo.log   ", "foo.log"},
		{"trailing tab", "foo.log\t", "foo.log"},
		{"mixed trailing", "foo.log \t ", "foo.log"},
		// Note: "build /" has space BEFORE slash, not trailing whitespace
		// After stripping trailing /, we get "build " which is valid
		// Git treats this as matching a directory named "build " (with space)
		{"space before slash", "build /", "build "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.line, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.line)
			}
			if len(r.segments) == 0 {
				t.Fatalf("parseLine(%q) has no segments", tt.line)
			}
			if r.segments[0].value != tt.wantSegValue {
				t.Errorf("parseLine(%q).segments[0].value = %q, want %q",
					tt.line, r.segments[0].value, tt.wantSegValue)
			}
		})
	}
}

func TestParseLine_Warnings(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantWarning bool
	}{
		{"valid pattern", "*.log", false},
		{"empty after negation", "!", true},
		{"empty after slash", "/", true},
		// "./" becomes "." after stripping trailing /, which is valid
		// (matches file/dir literally named ".")
		{"dot slash only", "./", false},
		{"negation with slash only", "!/", true},
		{"valid negation", "!important.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.line, 1, "")
			if tt.wantWarning {
				if w == nil {
					t.Errorf("parseLine(%q) should have warning", tt.line)
				}
				if r != nil {
					t.Errorf("parseLine(%q) should return nil rule with warning", tt.line)
				}
			} else {
				if w != nil {
					t.Errorf("parseLine(%q) unexpected warning: %v", tt.line, w)
				}
				if r == nil {
					t.Errorf("parseLine(%q) should return valid rule", tt.line)
				}
			}
		})
	}
}

func TestParseLine_LineNumber(t *testing.T) {
	r, _ := parseLine("*.log", 42, "")
	if r == nil {
		t.Fatal("parseLine returned nil")
	}
	if r.line != 42 {
		t.Errorf("r.line = %d, want 42", r.line)
	}

	_, w := parseLine("!", 17, "")
	if w == nil {
		t.Fatal("parseLine should return warning")
	}
	if w.Line != 17 {
		t.Errorf("w.Line = %d, want 17", w.Line)
	}
}

func TestParseLine_BasePath(t *testing.T) {
	r, _ := parseLine("*.log", 1, "src/lib")
	if r == nil {
		t.Fatal("parseLine returned nil")
	}
	if r.basePath != "src/lib" {
		t.Errorf("r.basePath = %q, want %q", r.basePath, "src/lib")
	}
}

func TestParseSegments(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		wantSegs []segment
	}{
		{
			"simple",
			"foo",
			[]segment{{value: "foo"}},
		},
		{
			"nested",
			"foo/bar",
			[]segment{{value: "foo"}, {value: "bar"}},
		},
		{
			"wildcard",
			"*.log",
			[]segment{{value: "*.log", wildcard: true}},
		},
		{
			"double star",
			"**",
			[]segment{{doubleStar: true}},
		},
		{
			"double star prefix",
			"**/foo",
			[]segment{{doubleStar: true}, {value: "foo"}},
		},
		{
			"double star suffix",
			"foo/**",
			[]segment{{value: "foo"}, {doubleStar: true}},
		},
		{
			"double star middle",
			"a/**/b",
			[]segment{{value: "a"}, {doubleStar: true}, {value: "b"}},
		},
		{
			"mixed wildcards",
			"src/*.go",
			[]segment{{value: "src"}, {value: "*.go", wildcard: true}},
		},
		{
			"complex",
			"a/**/b/*.txt",
			[]segment{
				{value: "a"},
				{doubleStar: true},
				{value: "b"},
				{value: "*.txt", wildcard: true},
			},
		},
		{
			"multiple wildcards in segment",
			"*test*.go",
			[]segment{{value: "*test*.go", wildcard: true}},
		},
		{
			"consecutive stars not double",
			"***.log",
			[]segment{{value: "***.log", wildcard: true}}, // Not a double-star
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSegments(tt.pattern)
			if len(got) != len(tt.wantSegs) {
				t.Fatalf("parseSegments(%q) = %d segments, want %d\ngot: %v",
					tt.pattern, len(got), len(tt.wantSegs), segmentsString(got))
			}
			for i, want := range tt.wantSegs {
				if got[i].value != want.value {
					t.Errorf("segment[%d].value = %q, want %q", i, got[i].value, want.value)
				}
				if got[i].wildcard != want.wildcard {
					t.Errorf("segment[%d].wildcard = %v, want %v", i, got[i].wildcard, want.wildcard)
				}
				if got[i].doubleStar != want.doubleStar {
					t.Errorf("segment[%d].doubleStar = %v, want %v", i, got[i].doubleStar, want.doubleStar)
				}
			}
		})
	}
}

func TestParseLines(t *testing.T) {
	content := []byte(`# Comment
*.log
build/

!important.log
src/temp
**/cache
`)

	rules, warnings := parseLines("", content)

	if len(warnings) != 0 {
		t.Errorf("parseLines returned %d warnings, want 0", len(warnings))
	}

	// Should have 5 rules (comment and empty line skipped)
	if len(rules) != 5 {
		t.Fatalf("parseLines returned %d rules, want 5", len(rules))
	}

	// Verify each rule
	expected := []struct {
		pattern  string
		negate   bool
		dirOnly  bool
		anchored bool
		line     int
	}{
		{"*.log", false, false, false, 2},
		{"build/", false, true, false, 3},
		{"!important.log", true, false, false, 5},
		{"src/temp", false, false, true, 6}, // anchored due to /
		{"**/cache", false, false, false, 7},
	}

	for i, exp := range expected {
		r := rules[i]
		if r.pattern != exp.pattern {
			t.Errorf("rule[%d].pattern = %q, want %q", i, r.pattern, exp.pattern)
		}
		if r.negate != exp.negate {
			t.Errorf("rule[%d].negate = %v, want %v", i, r.negate, exp.negate)
		}
		if r.dirOnly != exp.dirOnly {
			t.Errorf("rule[%d].dirOnly = %v, want %v", i, r.dirOnly, exp.dirOnly)
		}
		if r.anchored != exp.anchored {
			t.Errorf("rule[%d].anchored = %v, want %v", i, r.anchored, exp.anchored)
		}
		if r.line != exp.line {
			t.Errorf("rule[%d].line = %d, want %d", i, r.line, exp.line)
		}
	}
}

func TestParseLines_WithWarnings(t *testing.T) {
	content := []byte(`*.log
!
/
valid.txt
`)

	rules, warnings := parseLines("", content)

	// Should have 2 warnings (! and / become empty)
	if len(warnings) != 2 {
		t.Errorf("parseLines returned %d warnings, want 2", len(warnings))
		for _, w := range warnings {
			t.Logf("  warning: line %d: %s", w.Line, w.Message)
		}
	}

	// Should have 2 valid rules
	if len(rules) != 2 {
		t.Errorf("parseLines returned %d rules, want 2", len(rules))
	}
}

func TestParseLines_CRLF(t *testing.T) {
	// Windows line endings
	content := []byte("*.log\r\nbuild/\r\n!important.log\r\n")

	rules, warnings := parseLines("", content)

	if len(warnings) != 0 {
		t.Errorf("parseLines returned warnings: %v", warnings)
	}
	if len(rules) != 3 {
		t.Errorf("parseLines returned %d rules, want 3", len(rules))
	}
}

func TestParseLines_BOM(t *testing.T) {
	// UTF-8 BOM
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.log\nbuild/\n")...)

	rules, warnings := parseLines("", content)

	if len(warnings) != 0 {
		t.Errorf("parseLines returned warnings: %v", warnings)
	}
	if len(rules) != 2 {
		t.Errorf("parseLines returned %d rules, want 2", len(rules))
	}
	// First rule should be *.log, not BOM bytes
	if rules[0].segments[0].value != "*.log" {
		t.Errorf("first rule value = %q, want %q", rules[0].segments[0].value, "*.log")
	}
}

func TestParseLines_WithBasePath(t *testing.T) {
	content := []byte("*.log\ntemp/\n")

	rules, _ := parseLines("src/lib", content)

	for _, r := range rules {
		if r.basePath != "src/lib" {
			t.Errorf("rule.basePath = %q, want %q", r.basePath, "src/lib")
		}
	}
}

func TestSegmentMethods(t *testing.T) {
	tests := []struct {
		seg          segment
		isDoubleStar bool
		isWildcard   bool
		isLiteral    bool
	}{
		{segment{value: "foo"}, false, false, true},
		{segment{value: "*.log", wildcard: true}, false, true, false},
		{segment{doubleStar: true}, true, false, false},
	}

	for _, tt := range tests {
		if tt.seg.isDoubleStar() != tt.isDoubleStar {
			t.Errorf("segment(%+v).isDoubleStar() = %v, want %v",
				tt.seg, tt.seg.isDoubleStar(), tt.isDoubleStar)
		}
		if tt.seg.isWildcard() != tt.isWildcard {
			t.Errorf("segment(%+v).isWildcard() = %v, want %v",
				tt.seg, tt.seg.isWildcard(), tt.isWildcard)
		}
		if tt.seg.isLiteral() != tt.isLiteral {
			t.Errorf("segment(%+v).isLiteral() = %v, want %v",
				tt.seg, tt.seg.isLiteral(), tt.isLiteral)
		}
	}
}

// TestAnchoringExamplesFromSpec tests all examples from Section 7.2 of the spec
func TestAnchoringExamplesFromSpec(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		wantAnchored bool
		description  string
	}{
		// From spec Section 7.2
		{
			name:         "anchored contains slash",
			pattern:      "src/temp",
			wantAnchored: true,
			description:  "Contains / so anchored",
		},
		{
			name:         "not anchored plain name",
			pattern:      "temp",
			wantAnchored: false,
			description:  "Plain name, no /",
		},
		{
			name:         "anchored leading slash",
			pattern:      "/temp",
			wantAnchored: true,
			description:  "Leading / anchors to root",
		},
		{
			name:         "not anchored trailing slash",
			pattern:      "temp/",
			wantAnchored: false,
			description:  "Trailing / only means directory",
		},
		{
			name:         "not anchored doublestar prefix",
			pattern:      "**/temp",
			wantAnchored: false,
			description:  "**/ prefix makes it float",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.pattern, 1, "")
			if w != nil {
				t.Fatalf("unexpected warning: %v", w)
			}
			if r == nil {
				t.Fatal("rule is nil")
			}
			if r.anchored != tt.wantAnchored {
				t.Errorf("%s: anchored = %v, want %v\npattern: %q",
					tt.description, r.anchored, tt.wantAnchored, tt.pattern)
			}
		})
	}
}

func TestRuleString(t *testing.T) {
	tests := []struct {
		rule rule
		want string
	}{
		{
			rule: rule{pattern: "*.log"},
			want: "*.log",
		},
		{
			rule: rule{pattern: "!important.log", negate: true},
			want: "!important.log [negate]",
		},
		{
			rule: rule{pattern: "build/", dirOnly: true},
			want: "build/ [dirOnly]",
		},
		{
			rule: rule{pattern: "/root", anchored: true},
			want: "/root [anchored]",
		},
		{
			rule: rule{pattern: "*.log", basePath: "src"},
			want: "*.log @src",
		},
		{
			rule: rule{pattern: "!build/", negate: true, dirOnly: true, basePath: "lib"},
			want: "!build/ [negate,dirOnly] @lib",
		},
	}

	for _, tt := range tests {
		got := tt.rule.String()
		if got != tt.want {
			t.Errorf("rule.String() = %q, want %q", got, tt.want)
		}
	}
}