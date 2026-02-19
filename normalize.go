// Package ignore provides gitignore pattern matching for file paths.
package ignore

import (
	"bytes"
	"runtime"
	"strings"
)

// normalizePath normalizes a file path for consistent matching.
// It converts Windows-style paths to Unix-style and removes redundant elements.
//
// Normalization steps (applied in order):
//  1. Convert backslashes to forward slashes (Windows only — on Linux, \ is valid in filenames)
//  2. Collapse consecutive slashes
//  3. Remove leading "./" prefix (all occurrences for idempotency)
//  4. Remove trailing slash
//
// This function is applied to input paths (in Match/MatchWithReason) and base
// paths (in parseLines). It is NOT applied to patterns during parsing — patterns
// are parsed as-is and matched with their original escape sequences intact.
func normalizePath(p string) string {
	// Step 1: Convert backslashes to forward slashes (Windows only).
	// On Linux/macOS, backslash is a valid filename character and should not
	// be converted. Git only performs this conversion on Windows.
	if runtime.GOOS == "windows" {
		p = strings.ReplaceAll(p, "\\", "/")
	}

	// Step 2: Collapse consecutive slashes (must happen before trailing slash removal)
	if strings.Contains(p, "//") {
		var b strings.Builder
		b.Grow(len(p))
		prevSlash := false
		for i := 0; i < len(p); i++ {
			if p[i] == '/' {
				if !prevSlash {
					b.WriteByte('/')
				}
				prevSlash = true
			} else {
				b.WriteByte(p[i])
				prevSlash = false
			}
		}
		p = b.String()
	}

	// Step 3: Remove leading ./ (all occurrences for idempotency)
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}

	// Step 4: Remove trailing slash
	p = strings.TrimSuffix(p, "/")

	return p
}

// normalizeBasePath normalizes a base path for rule scoping.
// Similar to normalizePath but preserves the semantic meaning of basePath.
//
// The basePath represents the directory containing a .gitignore file,
// relative to the repository root. Empty string means repository root.
func normalizeBasePath(basePath string) string {
	if basePath == "" {
		return ""
	}

	// Apply standard path normalization (already removes trailing slash)
	basePath = normalizePath(basePath)

	return basePath
}

// normalizeContent normalizes .gitignore file content for parsing.
// It handles platform-specific encoding variations.
//
// Normalization steps (applied in order):
//  1. Strip UTF-8 BOM if present (EF BB BF) - loops for idempotency
//  2. Normalize CRLF to LF (Windows line endings)
//  3. Normalize standalone CR to LF (old Mac format)
//
// This ensures consistent parsing regardless of the file's origin platform.
func normalizeContent(content []byte) []byte {
	if len(content) == 0 {
		return content
	}

	// Step 1: Strip UTF-8 BOM if present (EF BB BF)
	// Loop to handle edge case of multiple BOMs for idempotency
	for len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		content = content[3:]
	}

	// Step 2: Normalize CRLF to LF (Windows line endings)
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))

	// Step 3: Handle standalone CR (old Mac format)
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))

	return content
}

// trimTrailingWhitespace removes trailing spaces and tabs from a line,
// respecting backslash-escaped spaces per the gitignore spec.
//
// Git behavior: "Trailing spaces are ignored unless they are quoted with backslash."
// A backslash before a trailing space preserves that space:
//   - "foo "    → "foo"    (trailing space stripped)
//   - "foo\ "   → "foo "   (escaped space preserved, backslash removed)
//   - "foo\\ "  → "foo\\"  (escaped backslash, unescaped trailing space stripped)
//   - "foo\\\ " → "foo\\ " (escaped backslash + escaped space preserved)
//
// Note: This does not trim newlines; those are handled during line splitting.
func trimTrailingWhitespace(line string) string {
	// Find end of non-whitespace content
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}

	if end == len(line) {
		return line // No trailing whitespace
	}

	// Count consecutive backslashes immediately before the whitespace
	bs := 0
	for i := end - 1; i >= 0 && line[i] == '\\'; i-- {
		bs++
	}

	// Odd number of backslashes means the last one escapes the first space
	if bs%2 == 1 && line[end] == ' ' {
		// Remove the escaping backslash, keep the space
		return line[:end-1] + " "
	}

	return line[:end]
}
