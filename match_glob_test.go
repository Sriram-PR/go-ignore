package ignore

import (
	"fmt"
	"strings"
	"testing"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// No wildcards
		{"foo", "foo", true},
		{"foo", "bar", false},

		// Simple star
		{"*", "", true},
		{"*", "anything", true},

		// Prefix
		{"foo*", "foo", true},
		{"foo*", "foobar", true},
		{"foo*", "fo", false},

		// Suffix
		{"*bar", "bar", true},
		{"*bar", "foobar", true},
		{"*bar", "ba", false},

		// Middle
		{"f*o", "fo", true},
		{"f*o", "foo", true},
		{"f*o", "fxxo", true},
		{"f*o", "fx", false},

		// Multiple stars
		{"*a*", "a", true},
		{"*a*", "ba", true},
		{"*a*", "ab", true},
		{"*a*", "bab", true},
		{"*a*", "b", false},

		// Consecutive stars inside a segment act as one *. A segment of
		// only stars is ** (TestMatchSegments_AllStars).
		{"f**o", "fo", true},
		{"f**o", "fxxxo", true},

		// Edge cases
		{"", "", true},
		{"", "x", false},
		{"*", "", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXbYcZ", false},

		// ? wildcard (matches exactly one character)
		{"?", "a", true},
		{"?", "", false},
		{"?", "ab", false},
		{"?.txt", "a.txt", true},
		{"?.txt", "ab.txt", false},
		{"?.txt", ".txt", false},
		{"foo?", "foob", true},
		{"foo?", "foo", false},
		{"f?o", "fao", true},
		{"f?o", "fo", false},
		{"f?o", "fabo", false},
		{"???", "abc", true},
		{"???", "ab", false},
		{"???", "abcd", false},

		// Mixed * and ?
		{"*?.log", "a.log", true},
		{"*?.log", "abc.log", true},
		{"*?.log", ".log", false}, // ? requires at least one char
		{"?*", "a", true},         // ? matches one, * matches zero
		{"?*", "abc", true},       // ? matches one, * matches two
		{"?*", "", false},         // ? requires at least one char
		{"?*?", "ab", true},       // ? matches one each side, * matches zero
		{"?*?", "a", false},       // Need at least 2 chars

		// ? with multi-byte Unicode (? matches one byte, not one rune)
		{"?", "é", false},   // é is 2 bytes, ? matches 1 byte
		{"??", "é", true},   // 2 bytes, ?? matches both
		{"???", "é", false}, // too many ?s for 2-byte char

		// Backslash escaping
		{"\\*", "*", true},              // \* matches literal *
		{"\\*", "a", false},             // \* does not match regular char
		{"\\?", "?", true},              // \? matches literal ?
		{"\\?", "a", false},             // \? does not match regular char
		{"\\\\", "\\", true},            // \\ matches literal backslash
		{"\\\\", "a", false},            // \\ does not match regular char
		{"foo\\*bar", "foo*bar", true},  // Escaped * in middle
		{"foo\\*bar", "fooXbar", false}, // Escaped * is literal, not wildcard
		{"\\*.txt", "*.txt", true},      // Escaped * then literal
		{"\\*.txt", "a.txt", false},     // Escaped * is literal
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.s), func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.s)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v",
					tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

func TestMatchGlob_CharClass(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// Basic classes
		{"[abc]", "a", true},
		{"[abc]", "b", true},
		{"[abc]", "c", true},
		{"[abc]", "d", false},
		{"[abc]", "", false},

		// Ranges
		{"[a-z]", "a", true},
		{"[a-z]", "m", true},
		{"[a-z]", "z", true},
		{"[a-z]", "A", false},
		{"[a-z]", "0", false},
		{"[0-9]", "0", true},
		{"[0-9]", "5", true},
		{"[0-9]", "9", true},
		{"[0-9]", "a", false},
		{"[a-zA-Z0-9]", "a", true},
		{"[a-zA-Z0-9]", "Z", true},
		{"[a-zA-Z0-9]", "5", true},
		{"[a-zA-Z0-9]", "!", false},

		// Negation with !
		{"[!abc]", "d", true},
		{"[!abc]", "a", false},
		{"[!abc]", "z", true},
		{"[!0-9]", "a", true},
		{"[!0-9]", "5", false},

		// Negation with ^
		{"[^abc]", "d", true},
		{"[^abc]", "a", false},

		// Literal hyphen at start/end
		{"[-abc]", "-", true},
		{"[-abc]", "a", true},
		{"[-abc]", "d", false},
		{"[abc-]", "-", true},
		{"[abc-]", "a", true},
		{"[abc-]", "d", false},

		// Literal ] as first member
		{"[]abc]", "]", true},
		{"[]abc]", "a", true},
		{"[]abc]", "d", false},

		// Negated with ] as first
		{"[!]abc]", "]", false},
		{"[!]abc]", "d", true},

		// Escapes inside class
		{"[\\[]", "[", true},
		{"[\\]]", "]", true},
		{"[a\\-z]", "a", true},
		{"[a\\-z]", "-", true},
		{"[a\\-z]", "z", true},
		{"[a\\-z]", "m", false},

		// Unclosed bracket — treated as literal
		{"[abc", "[", false}, // literal '[' + "abc" — doesn't match
		{"[abc", "a", false},

		// Slash exclusion (FNM_PATHNAME)
		{"[/]", "/", false},
		{"[!a]", "/", false}, // Negated class still can't match /

		// Integration with other wildcards
		{"*.[ch]", "foo.c", true},
		{"*.[ch]", "foo.h", true},
		{"*.[ch]", "foo.o", false},
		{"[abc]*.go", "atest.go", true},
		{"[abc]*.go", "dtest.go", false},
		{"?[0-9]", "a5", true},
		{"?[0-9]", "ab", false},
		{"?[0-9]", "55", true},

		// Escaped range endpoints
		{"[\\a-z]", "m", true},
		{"[\\a-z]", "a", true},
		{"[\\a-z]", "z", true},
		{"[a-\\z]", "m", true},
		{"[a-\\z]", "a", true},
		{"[a-\\z]", "z", true},

		// Reversed range: git compares the start as a plain member before it
		// sees the '-' (wildmatch.c), so only the start matches.
		{"[z-a]", "m", false},
		{"[z-a]", "a", false},
		{"[z-a]", "z", true},
		{"[b-a]x", "bx", true},
		{"[b-a]x", "ax", false},
		{"[a-\\]]x", "ax", true},
		{"[a-\\]]x", "]x", false},
		{"[a-\\]]x", "bx", false},
		{"[\\z-a]x", "zx", true},

		// Wildcards inside class are literal
		{"[*?]", "*", true},
		{"[*?]", "?", true},
		{"[*?]", "a", false},

		// Inner '[' is literal
		{"[[abc]", "[", true},
		{"[[abc]", "a", true},
		{"[[abc]", "d", false},

		// POSIX classes
		{"[[:alpha:]]", "a", true},
		{"[[:alpha:]]", "Z", true},
		{"[[:alpha:]]", "5", false},
		{"[[:digit:]]", "5", true},
		{"[[:digit:]]", "a", false},
		{"[![:digit:]]", "a", true},
		{"[![:digit:]]", "5", false},
		{"[[:upper:]]", "A", true},
		{"[[:upper:]]", "a", false},
		{"[[:lower:]]", "a", true},
		{"[[:lower:]]", "A", false},
		{"[[:alnum:]]", "a", true},
		{"[[:alnum:]]", "5", true},
		{"[[:alnum:]]", "!", false},
		{"[[:xdigit:]]", "f", true},
		{"[[:xdigit:]]", "F", true},
		{"[[:xdigit:]]", "g", false},
		{"[[:space:]]", " ", true},
		{"[[:space:]]", "\t", true},
		{"[[:space:]]", "a", false},

		// POSIX blank
		{"[[:blank:]]", " ", true},
		{"[[:blank:]]", "\t", true},
		{"[[:blank:]]", "a", false},
		{"[[:blank:]]", "\n", false},

		// POSIX print
		{"[[:print:]]", "a", true},
		{"[[:print:]]", " ", true},
		{"[[:print:]]", "~", true},
		{"[[:print:]]", "\x01", false},
		{"[[:print:]]", "\x7f", false},

		// POSIX graph
		{"[[:graph:]]", "a", true},
		{"[[:graph:]]", "!", true},
		{"[[:graph:]]", " ", false},
		{"[[:graph:]]", "\x01", false},

		// POSIX punct
		{"[[:punct:]]", "!", true},
		{"[[:punct:]]", ".", true},
		{"[[:punct:]]", "@", true},
		{"[[:punct:]]", "{", true},
		{"[[:punct:]]", "a", false},
		{"[[:punct:]]", "0", false},

		// POSIX cntrl
		{"[[:cntrl:]]", "\x01", true},
		{"[[:cntrl:]]", "\x7f", true},
		{"[[:cntrl:]]", "a", false},
		{"[[:cntrl:]]", " ", false},

		// Invalid POSIX name — '[' treated as literal class member, rest parsed as literals.
		// Effective class contains: [, :, b, o, g, u, s. Trailing ']' is literal outside class.
		{"[[:bogus:]]", "[", false},  // needs 2 chars (class member + literal ']')
		{"[[:bogus:]]", "[]", true},  // '[' in class, ']' matches trailing literal
		{"[[:bogus:]]", "b]", true},  // 'b' in class (from "bogus"), ']' matches
		{"[[:bogus:]]", "x]", false}, // 'x' not in class

		// Mixed POSIX and literals/ranges
		{"[[:digit:]abc]", "5", true},
		{"[[:digit:]abc]", "a", true},
		{"[[:digit:]abc]", "z", false},
		{"[a-c[:upper:]]", "b", true},
		{"[a-c[:upper:]]", "Z", true},
		{"[a-c[:upper:]]", "z", false},

		// Multiple classes in a pattern
		{"[abc][0-9]", "a5", true},
		{"[abc][0-9]", "d5", false},
		{"[abc][0-9]", "a", false},

		// Class with file extensions
		{"[Mm]akefile", "Makefile", true},
		{"[Mm]akefile", "makefile", true},
		{"[Mm]akefile", "Xakefile", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.s), func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.s)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v",
					tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

// The byte loop gives the right answer on the classic exponential inputs: a
// match when one exists and no match otherwise.
func TestMatchGlob_PathologicalPatternCorrect(t *testing.T) {
	pattern := strings.Repeat("*a", 15) + "*b"
	if matchGlob(pattern, strings.Repeat("a", 30)) {
		t.Error("no b in input: want no match")
	}
	if !matchGlob(pattern, strings.Repeat("a", 30)+"b") {
		t.Error("15 a's then b: want a match")
	}
}

func TestClassEnd(t *testing.T) {
	tests := []struct {
		pattern string
		end     int
		problem string
	}{
		{"[abc]x", 5, ""},
		{"[]a]x", 4, ""},
		{"[!]a]x", 5, ""},
		{"[]]", 3, ""},
		{"[\\]]x", 4, ""},
		{"[[:alpha:]]x", 11, ""},
		{"[[:]]x", 4, ""},          // "[:]" is not a class name: '[' and ':' are members
		{"[[:a]b:]]", 5, ""},       // first ']' after "[:" is not preceded by ':'
		{"[a-[:alpha:]]x", 12, ""}, // '[' ends the range, so "[:" starts nothing
		{"[[:alpha:]-z]x", 13, ""},
		{"[a/b]x", 5, ""},
		{"[", 0, problemUnclosedClass},
		{"[]", 0, problemUnclosedClass},
		{"[!]", 0, problemUnclosedClass},
		{"[abc", 0, problemUnclosedClass},
		{"[a\\", 0, problemUnclosedClass},
		{"[[:alpha:]", 0, problemUnclosedClass},
		{"[[:alpha", 0, problemUnclosedClass},
		{"[[:foo:]]", 0, problemUnknownClass},
		{"[[::]]", 0, problemUnknownClass},
	}
	for _, tt := range tests {
		end, problem := classEnd(tt.pattern, 0)
		if end != tt.end || problem != tt.problem {
			t.Errorf("classEnd(%q) = %d, %q, want %d, %q", tt.pattern, end, problem, tt.end, tt.problem)
		}
	}
}
