// Package ignore provides gitignore pattern matching for file paths.
package ignore

import (
	"bytes"
	"strings"
)

// normalizePath normalizes a file path for consistent matching.
// It converts Windows-style paths to Unix-style and removes redundant elements.
//
// Normalization steps (applied in order):
//  1. Convert backslashes to forward slashes (Windows compatibility)
//  2. Collapse consecutive slashes
//  3. Remove leading "./" prefix (all occurrences for idempotency)
//  4. Remove trailing slash
//
// This function is applied to both patterns (during parsing) and input paths
// (in Match), ensuring consistent behavior regardless of path style.
func normalizePath(p string) string {
	// Step 1: Convert backslashes to forward slashes (Windows)
	p = strings.ReplaceAll(p, "\\", "/")

	// Step 2: Collapse double slashes (must happen before trailing slash removal)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
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

	// Apply standard path normalization
	basePath = normalizePath(basePath)

	// Ensure no trailing slash for consistent prefix matching
	basePath = strings.TrimSuffix(basePath, "/")

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

// trimTrailingWhitespace removes trailing spaces and tabs from a line.
// This matches Git's behavior of ignoring trailing whitespace in patterns.
// Note: This does not trim newlines; those are handled during line splitting.
func trimTrailingWhitespace(line string) string {
	return strings.TrimRight(line, " \t")
}
