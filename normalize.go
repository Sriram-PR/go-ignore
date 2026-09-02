package ignore

import (
	"bytes"
	"path"
	"runtime"
	"strings"
)

// normalizePath canonicalizes a path for matching: on Windows backslashes
// become slashes; repeated and trailing slashes and leading "./" are removed;
// and "."/".." are resolved via path.Clean so "src/../secret.txt" cannot bypass
// a "src/"-scoped rule. A path containing a null byte, or one resolving above
// the root, returns "".
//
// Applied to input and base paths, never to patterns — patterns keep their
// original escape sequences.
func normalizePath(p string) string {
	if strings.IndexByte(p, 0) >= 0 {
		return ""
	}

	// On Linux/macOS backslash is a valid filename character; git converts it
	// to a slash only on Windows.
	if runtime.GOOS == "windows" {
		p = strings.ReplaceAll(p, "\\", "/")
	}

	// Collapse repeated slashes before trailing-slash removal.
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

	for strings.HasPrefix(p, "./") { // repeat for idempotency
		p = p[2:]
	}

	p = strings.TrimSuffix(p, "/")

	// path.Clean resolves "."/".." so a path can't climb out of its scope; one
	// resolving to the root or above (".", "..", "../…") is treated as no match.
	if strings.Contains(p, "..") || strings.Contains(p, "/./") {
		p = path.Clean(p)
		if p == "." || p == "/" {
			return ""
		}
		if p == ".." || strings.HasPrefix(p, "../") {
			return ""
		}
	}

	return p
}

// normalizeContent prepares .gitignore bytes for parsing: it strips a UTF-8 BOM
// and converts CRLF/CR line endings to LF, so files authored on any platform
// parse identically.
func normalizeContent(content []byte) []byte {
	if len(content) == 0 {
		return content
	}

	// The loop handles the (degenerate) case of repeated BOMs for idempotency.
	for len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		content = content[3:]
	}

	if bytes.IndexByte(content, '\r') < 0 {
		return content // fast path: no CR at all
	}
	buf := make([]byte, 0, len(content))
	for i := 0; i < len(content); i++ {
		if content[i] == '\r' {
			buf = append(buf, '\n')
			if i+1 < len(content) && content[i+1] == '\n' {
				i++ // skip the \n after \r
			}
		} else {
			buf = append(buf, content[i])
		}
	}
	return buf
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
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}

	if end == len(line) {
		return line // No trailing whitespace
	}

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
