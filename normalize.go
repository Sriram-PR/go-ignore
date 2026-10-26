package ignore

import (
	"bytes"
	"path"
	"runtime"
	"strings"
)

// normalizePath canonicalizes a path for matching: on Windows backslashes
// become slashes; repeated, leading and trailing slashes and leading "./" are
// removed, so "/sub/x" names the same path as "sub/x";
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

	// Before path.Clean, so "/../x" stays above the root like "../x".
	for { // repeat for idempotency
		if strings.HasPrefix(p, "/") {
			p = p[1:]
		} else if strings.HasPrefix(p, "./") {
			p = p[2:]
		} else {
			break
		}
	}

	p = strings.TrimSuffix(p, "/")

	// path.Clean resolves "."/".." so a path can't climb out of its scope, and
	// so a . segment is not matched as a name; one resolving to the root or
	// above (".", "..", "../…") is treated as no match.
	if strings.Contains(p, "..") || strings.Contains(p, "/./") || p == "." || strings.HasSuffix(p, "/.") {
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

// baseOutsideRoot reports whether basePath, which normalizePath turned into
// "", names something other than the root: it has a null byte or resolves
// above the root.
func baseOutsideRoot(basePath string) bool {
	if strings.IndexByte(basePath, 0) >= 0 {
		return true
	}
	p := basePath
	if runtime.GOOS == "windows" {
		p = strings.ReplaceAll(p, "\\", "/")
	}
	p = path.Clean(strings.TrimLeft(p, "/"))
	return p == ".." || strings.HasPrefix(p, "../")
}

// normalizeContent prepares .gitignore bytes for parsing the way git's
// add_patterns does: it strips one leading UTF-8 BOM and drops a CR that
// precedes LF or ends the content. Any other CR is pattern text.
func normalizeContent(content []byte) []byte {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	if bytes.IndexByte(content, '\r') < 0 {
		return content // fast path: no CR at all
	}
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	return bytes.TrimSuffix(content, []byte("\r"))
}

// trimTrailingWhitespace removes trailing spaces (not tabs, matching git) from
// a line, respecting backslash-escaped spaces per the gitignore spec.
//
// Git behavior: "Trailing spaces are ignored unless they are quoted with backslash."
// A backslash before a trailing space keeps that space, and the backslash
// stays too, as in git's trim_trailing_spaces: matching reads "\ " as a
// literal space, and MatchResult.Rule shows the line as git check-ignore -v
// does.
//   - "foo "    → "foo"    (trailing space stripped)
//   - "foo\ "   → "foo\ "   (escaped space kept)
//   - "foo\\ "  → "foo\\"  (escaped backslash, unescaped trailing space stripped)
//   - "foo\\\ " → "foo\\\ " (escaped backslash + escaped space kept)
//
// Note: This does not trim newlines; those are handled during line splitting.
func trimTrailingWhitespace(line string) string {
	end := len(line)
	for end > 0 && line[end-1] == ' ' {
		end--
	}

	if end == len(line) {
		return line // No trailing whitespace
	}

	// Odd number of backslashes means the last one escapes the first space
	if trailingBackslashes(line[:end])%2 == 1 {
		return line[:end+1]
	}

	return line[:end]
}

// trailingBackslashes counts the backslashes at the end of s. An odd count
// means the last one escapes whatever follows it.
func trailingBackslashes(s string) int {
	n := 0
	for n < len(s) && s[len(s)-1-n] == '\\' {
		n++
	}
	return n
}
