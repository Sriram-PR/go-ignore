package ignore_test

import (
	"fmt"
	"math/rand"
	"path"
	"strings"
)

// scenario is one parity case: .gitignore files keyed by slash path relative
// to the repo root, and every path to check mapped to whether it is a
// directory.
type scenario struct {
	name  string
	icase bool
	files map[string]string
	paths map[string]bool
}

type curatedCase struct {
	name  string
	files map[string]string
	paths []string // trailing "/" = dir
	icase bool
}

func curatedRoot(c string, paths ...string) curatedCase {
	return curatedCase{files: map[string]string{".gitignore": c}, paths: paths}
}

func curatedCases() []curatedCase {
	var cs []curatedCase
	add := func(c curatedCase) { cs = append(cs, c) }
	R := curatedRoot
	// trailing whitespace / escapes
	add(R("foo\t\n", "foo", "foo\t"))
	add(R("foo \n", "foo", "foo "))
	add(R("foo\\ \n", "foo", "foo "))
	add(R("foo\\  \n", "foo", "foo "))
	add(R("foo\\\\ \n", "foo", "foo\\", "foo\\ "))
	add(R("foo\\\\\\ \n", "foo", "foo\\ ", "foo\\\\"))
	add(R("foo \t \n", "foo"))
	add(R(" foo\n", " foo", "foo"))
	add(R("   \nfoo\n", "foo", "x"))
	add(R("foo\\\t\n", "foo\t", "foo"))
	add(R("foo\\\n", "foo", "foo\\"))
	add(R("foo\\\\\n", "foo\\", "foo"))
	add(R("a\\ b\n", "a b"))
	add(R("a b\n", "a b"))
	add(R("a  \\ \n", "a  ", "a"))
	// line endings / BOM
	add(R("foo\r\nbar\r\n", "foo", "bar", "foo\r"))
	add(R("foo\rbar\n", "foo", "bar", "foo\rbar"))
	add(R("foo\r", "foo", "foo\r"))
	add(R("\r\nfoo\r\n", "foo"))
	add(R("foo\r \n", "foo", "foo\r"))
	add(R("foo \r\n", "foo", "foo "))
	add(R("\xef\xbb\xbffoo\n", "foo"))
	add(R("\xef\xbb\xbf\xef\xbb\xbffoo\n", "foo", "\xef\xbb\xbffoo"))
	add(R("bar\n\xef\xbb\xbffoo\n", "foo", "\xef\xbb\xbffoo", "bar"))
	add(R("\xef\xbb\xbf#c\nfoo\n", "foo"))
	add(R("\xef\xbb\xbf!foo\n*\n", "foo", "bar"))
	// comments / negation prefix
	add(R("#foo\n", "#foo"))
	add(R("\\#foo\n", "#foo"))
	add(R(" #foo\n", " #foo", "#foo"))
	add(R("!#foo\n*\n", "#foo", "x"))
	add(R("*\n!\\#foo\n", "#foo", "x"))
	add(R("\\!foo\n", "!foo", "foo"))
	add(R("!\\!foo\n*\n", "!foo", "x"))
	add(R("\\\\!foo\n", "\\!foo", "!foo"))
	add(R("!!foo\n*\n", "!foo", "foo"))
	add(R("!\n*\n", "x", "!"))
	add(R("\\\n", "x"))
	add(R("/\n", "x", "d/"))
	add(R("//\n", "x", "d/"))
	add(R("!/\n*\n", "x"))
	add(R("/ \n", "x"))
	add(R("a//b\n", "a/", "a/b"))
	add(R("//a\n", "a"))
	add(R("a//\n", "a/"))
	add(R("a///b\n", "a/", "a/b"))
	// ** positions
	for _, p := range []string{"**/foo", "foo/**", "a/**/b", "**", "/**", "**/", "/**/", "a**", "**a", "a/**/**/b", "**/**/b", "a/**/", "a/**b", "a**/b", "**/a/**", "***", "***/b", "a/***/b", "a/***", "a/****/b", "**/**", "a/**/**", "/a/**/b/**", "**/a**", "a/**/b/", "**foo", "**/*.log", "a/**/*.log", "\\**", "**\\/a", "*\\*", "**/\\*"} {
		add(R(p+"\n", "foo", "foo/", "a/", "a/b", "a/b/", "a/x/b", "a/x/y/b", "a/x/y/b/", "b", "ab", "ab/", "a/x.log", "x.log", "a/x/", "foo/a/", "foo/a/x", "a/x/b/c", "a/b/c/", "*", "a*", "a*/b", "*/", "a/b/c/d", "a/x/y/", "z/a/x/y/"))
	}
	// dir-only and negation
	add(R("build/\n", "build", "build/", "build/x", "x/build/", "x/build/y", "x/build"))
	add(R("*\n!*/\n", "a/", "a/x", "x", "a/b/", "a/b/y"))
	add(R("*\n!*/\n!*.go\n", "a/", "a/x.go", "a/x", "x.go", "a/b/", "a/b/y.go", "a/b/y"))
	add(R("*\n!build/\n", "build/", "build/x", "x", "d/build/", "d/build/x"))
	add(R("/*\n!/foo\n/foo/*\n!/foo/bar\n", "foo/", "foo/x", "foo/bar", "foo/bar/", "foo/bar/z", "other"))
	add(R("a/\n!a/b\n", "a/", "a/b", "a/c"))
	add(R("a/*\n!a/b\n", "a/", "a/b", "a/b/", "a/b/c", "a/c", "a/c/d"))
	add(R("a/**\n!a/b\n", "a/", "a/b", "a/b/", "a/b/c", "a/c"))
	add(R("**/a/\n!**/a/b\n", "a/", "a/b", "x/a/", "x/a/b"))
	add(R("a\n!a/b\n", "a/", "a/b", "a"))
	add(R("a\n!a\n", "a", "a/"))
	add(R("*.log\n!important.log\nimportant.log\n", "important.log", "x.log"))
	add(R("!a\na\n", "a", "a/"))
	add(R("a/\n!a/\n", "a/", "a/x"))
	add(R("a/\n!a\n", "a/", "a/x"))
	add(R("a\n!a/\n", "a/", "a/x"))
	add(R("foo/\n", "foo"))
	add(R("foo/ \n", "foo/", "foo/x"))
	add(R("foo/\\ \n", "foo/", "foo /", "foo /x"))
	add(R("dir/*\n!dir/sub/\n", "dir/", "dir/sub/", "dir/sub/f", "dir/f", "dir/sub/deep/", "dir/sub/deep/f"))
	add(R("/a/b/\n", "a/", "a/b/", "a/b/c", "b/"))
	add(R("a/b/\n", "a/", "a/b/", "a/b/c", "x/a/b/", "x/a/b/c"))
	// anchoring
	add(R("a/b\n", "a/", "a/b", "x/a/", "x/a/b"))
	add(R("/a\n", "a", "x/", "x/a"))
	add(R("a\n", "a", "x/", "x/a", "x/y/", "x/y/a"))
	add(R("*/a\n", "x/", "x/a", "x/y/", "x/y/a", "a"))
	add(R("a/*\n", "a/", "a/b", "a/b/", "a/b/c"))
	add(R("a/*/c\n", "a/", "a/b/", "a/b/c", "a/b/x/", "a/b/x/c", "a/c"))
	add(R("*/\n", "a/", "a/b", "b"))
	add(R("*\n", "a", ".x", "d/", "d/e"))
	add(R("?\n", "a", "ab", "é", "\xff"))
	add(R("??\n", "é", "ab", "a", "\xff\xfe"))
	// brackets
	for _, p := range []string{"[a-c]x", "[!a-c]x", "[^a-c]x", "[]]x", "[]a]x", "[!]]x", "[a-]x", "[-a]x", "[a-c-e]x", "[a-a]x", "[b-a]x", "[z-a]x", "[[:alpha:]]x", "[[:digit:]]x", "[[:alpha:][:digit:]]x", "[[:alpha:]x", "[[:foo:]]x", "[[:alpha]]x", "[[:alpha:]-z]x", "[a-[:alpha:]]x", "[\\]]x", "[\\a]x", "[\\-]x", "[a\\-c]x", "[ac\\]x", "[x", "x[", "[]x", "[!]x", "[!x", "[a-", "[[]x", "[[x]x", "[\\\\]x", "[*]x", "[?]x", "[/]x", "a[/]x", "[a-z]", "[A-Z]x", "[[:upper:]]x", "[[:lower:]]x", "[[:punct:]]x", "[[:space:]]x", "[[:blank:]]x", "[[:cntrl:]]x", "[[:graph:]]x", "[[:print:]]x", "[[:xdigit:]]x", "[[:alnum:]]x", "[^]]x", "[\\!a]x", "[!!]x", "[a-\\]]x", "[+--]x", "[--0]x", "[%--]x", "[a-c]*", "*[a-c]", "x[0-9]*y", "[.]x", "[[.a.]]x", "[[=a=]]x", "[\xe9]x", "[\xe0-\xff]x", "?[a]"} {
		add(R(p+"\n", "ax", "bx", "cx", "dx", "-x", "]x", "[x", "!x", "^x", "\\x", "*x", "?x", "/x", "Ax", "1x", " x", ".x", "x", "[:x", "ex", "zx", "xx", "ax/", "a1y", "x", "\xe9x", "\xffx", "a[x", "a/x", "a/", "a]x", "a-x", "[a]x", "ab", "a", "b", "A", "+x", ",x", "--x", "0x", "!!x"))
	}
	// escapes in globs
	for _, p := range []string{"\\*", "\\?", "\\[a]", "\\a", "a\\b", "a\\\\b", "\\\\", "\\\\\\*", "*\\", "*\\*", "\\*\\*", "a\\/b", "a\\/", "\\/a", "a/\\/b", "\\", "\\\\\\", "\\a*", "*\\a", "x\\*y", "\\ ", "\\\t"} {
		add(R(p+"\n", "*", "?", "[a]", "a", "ab", "a\\b", "\\", "\\\\", "\\*", "a/b", "a/", "/a", "x*y", "xay", "a*", "a?", " ", "\t", "aa"))
	}
	// star matching
	for _, p := range []string{"a*b", "a*b*c", "*a*", "a*", "*a", "**", "a?b", "*?", "?*", "a*?b", "*.", ".*", "*.*", "a**b", "a***b", "*/*", "*/**", "**/*", "a/*/b"} {
		add(R(p+"\n", "ab", "axb", "axbxc", "a", "b", ".", ".x", "a.", "x.y", "a/b", "a/x/b", "a/", "a/x/", "a/x/y", "abc", "aab", "a.b", "..", "a/b/", "x/y"))
	}
	// non-ASCII / bytes
	add(R("é\n", "é", "e"))
	add(R("?\n", "é"))
	add(R("\xff\n", "\xff", "\xfe"))
	add(R("[\xff]\n", "\xff", "\xfe"))
	add(R("a?\n", "a\xff", "a\xc3\xa9", "ab"))
	add(R("a??\n", "a\xc3\xa9", "a\xe2\x82\xac"))
	add(R("日本*\n", "日本語", "日"))
	add(R("[日]\n", "日", "\xe6"))
	// icase
	ic := func(c curatedCase) curatedCase { c.icase = true; return c }
	add(ic(R("Foo\n", "foo", "FOO", "Foo", "fOo", "x")))
	add(ic(R("foo\n", "FOO", "Foo/")))
	add(ic(R("[A-Z]x\n", "ax", "Ax", "bx", "Bx")))
	add(ic(R("[a-z]x\n", "ax", "Ax", "Bx")))
	add(ic(R("[[:upper:]]x\n", "ax", "Ax")))
	add(ic(R("[[:lower:]]x\n", "ax", "Ax")))
	add(ic(R("[[:UPPER:]]x\n", "ax", "Ax")))
	add(ic(R("[!A]x\n", "ax", "Ax", "bx", "Bx")))
	add(ic(R("[!a]x\n", "ax", "Ax", "bx", "Bx")))
	add(ic(R("\\Fo*\n", "foo", "FOO")))
	add(ic(R("É\n", "é", "É", "e")))
	add(ic(R("é\n", "É", "é")))
	add(ic(R("a?\n", "a\xff", "A\xff", "AB")))
	add(ic(R("a\xff\n", "a\xff", "A\xff", "a\xfe", "A\xfe")))
	add(ic(R("\xff\n", "\xff", "\xfe")))
	add(ic(R("K\n", "k", "\xe2\x84\xaa")))
	add(ic(R("k\n", "K", "\xe2\x84\xaa")))
	add(ic(R("İ\n", "i", "İ", "i̇")))
	add(ic(R("ǅ\n", "ǆ", "ǅ", "Ǆ")))
	add(ic(R("A/B\n", "a/b", "A/b", "X/a/b")))
	add(ic(R("a/\n!A/Keep\n", "a/", "A/keep", "A/other")))
	add(ic(curatedCase{files: map[string]string{".gitignore": "*.LOG\n", "Sub/.gitignore": "!x.log\n"}, paths: []string{"x.log", "Sub/", "sub/", "Sub/x.log", "SUB/x.log"}}))
	// nested
	add(curatedCase{files: map[string]string{".gitignore": "*.log\n", "sub/.gitignore": "!keep.log\n/only\nb/c\n*.tmp\n"}, paths: []string{"a.log", "sub/", "sub/keep.log", "sub/a.log", "sub/only", "sub/x/", "sub/x/only", "sub/b/", "sub/b/c", "x/b/c", "sub/x/b/c", "sub/x/b/", "sub/a.tmp", "sub/x/a.tmp", "a.tmp", "sub/x/keep.log", "other/", "other/keep.log"}})
	add(curatedCase{files: map[string]string{".gitignore": "sub/\n", "sub/.gitignore": "!keep\n"}, paths: []string{"sub/", "sub/keep", "sub/other"}})
	add(curatedCase{files: map[string]string{".gitignore": "sub\n", "sub/.gitignore": "!keep\n"}, paths: []string{"sub/", "sub/keep", "sub/other"}})
	add(curatedCase{files: map[string]string{".gitignore": "sub/*\n!sub/keep/\n", "sub/keep/.gitignore": "!*\n"}, paths: []string{"sub/", "sub/keep/", "sub/keep/f", "sub/other"}})
	add(curatedCase{files: map[string]string{".gitignore": "/*\n!/sub/\n", "sub/.gitignore": "x\n!y\n", "sub/deep/.gitignore": "!x\n"}, paths: []string{"sub/", "sub/x", "sub/y", "sub/deep/", "sub/deep/x", "other/", "other/f", "top"}})
	add(curatedCase{files: map[string]string{"a/.gitignore": "**/z\n/b\nc/d\n**\n!e\n", "a/b/.gitignore": "!*\n"}, paths: []string{"a/", "a/z", "a/q/z", "a/b", "a/e", "a/f", "a/b/x", "a/c/", "a/c/d", "z"}})
	add(curatedCase{files: map[string]string{"a/.gitignore": "../b\n/../c\n./d\n", ".gitignore": ""}, paths: []string{"a/", "b", "c", "a/d", "a/b", "a/c"}})
	add(curatedCase{files: map[string]string{".gitignore": "./foo\n../bar\n.\n..\n./\n"}, paths: []string{"foo", "bar", ".x"}})
	add(curatedCase{files: map[string]string{"a b/.gitignore": "x\n", "é/.gitignore": "*.o\n"}, paths: []string{"a b/", "a b/x", "é/", "é/y.o", "y.o", "x"}})
	add(curatedCase{files: map[string]string{".gitignore": "*\n!.gitignore\n!sub/\n!sub/.gitignore\n", "sub/.gitignore": "!*.go\n"}, paths: []string{"sub/", "sub/a.go", "sub/a", "a.go"}})
	// trailing space/dir interplay
	add(R("foo/  \n", "foo/", "foo/x"))
	add(R("foo /\n", "foo /", "foo/"))
	// pattern-vs-directory
	add(R("a.b/\n", "a.b/", "a.b/x", "a.b"))
	add(R("*.d/\n", "x.d/", "x.d/f", "y/x.d/f", "x.d"))
	add(R("a/b/**\n", "a/b/", "a/b/c", "a/b/c/", "a/b/c/d", "a/", "x/a/b/c"))
	add(R("**/a/b\n", "a/", "a/b", "x/a/b", "x/y/a/b", "a/b/c"))
	add(R("a/**/b\n", "a/b", "a/b/", "a/x/b", "a/x/y/b", "x/a/b", "a/x/"))
	add(R("a/**/**/b\n", "a/b", "a/x/b", "a/x/y/b"))
	add(R("**/a\n!a/b\n", "a/", "a/b", "x/a/", "x/a/b"))
	return cs
}

var names = []string{"a", "b", "ab", "a.b", "foo", "bar", "Foo", "FOO", ".x", "a b", "é", "[a]", "a*", "-", "]", "!a", "#a", "a]", "x"}
var comps = []string{
	"a", "b", "ab", "foo", "bar", "Foo", "*", "?", "**", "a*", "*b", "*a*", "a?", "??", "[ab]", "[!a]", "[^a]", "[]a]", "[!]a]", "[a-b]", "[a-]", "[-a]",
	"[[:alpha:]]", "[[:upper:]]", "[[:lower:]]", "[[:digit:]]", "[[:punct:]]", "[[:space:]]", "[[:foo:]]", "[a-c-e]", "[\\]]", "[\\a]", "[a\\-c]",
	"\\*", "\\a", "\\?", "\\[a]", "a\\ ", "a\\", "*\\*", "a**", "**a", "a**b", "x**", "é", "?", "\\!a", "\\#a", ".x", "*.b", "a.*", "a\\ b", "a b", "[[:alpha:][:digit:]]", "[[:alpha:]-z]", "[a-[:alpha:]]", "[!-a]", "[--a]", "[%--]", "[\\\\]", "[a[:]]",
}

func genPattern(r *rand.Rand) string {
	var sb strings.Builder
	if r.Intn(6) == 0 {
		sb.WriteString("!")
	}
	if r.Intn(4) == 0 {
		sb.WriteString("/")
	}
	n := 1 + r.Intn(3)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString("/")

		}
		sb.WriteString(comps[r.Intn(len(comps))])
	}
	if r.Intn(4) == 0 && !strings.HasSuffix(sb.String(), "**") {
		sb.WriteString("/")
	}
	switch r.Intn(12) {
	case 0:
		sb.WriteString(" ")
	case 1:
		sb.WriteString(" ")
	case 2:
		sb.WriteString("  ")
	}
	return sb.String()
}

func genScenario(r *rand.Rand, icase bool) *scenario {
	s := &scenario{files: map[string]string{}, paths: map[string]bool{}}
	dirs := []string{"", "a", "foo", "a/b", "b/a", "foo/bar"}
	if icase {
		dirs = []string{"", "a", "foo", "a/b", "b/a", "foo/bar"}
	}
	nfiles := 1 + r.Intn(3)
	for i := 0; i < nfiles; i++ {
		d := dirs[r.Intn(len(dirs))]
		if i == 0 && r.Intn(2) == 0 {
			d = ""
		}
		var lines []string
		for j, np := 0, 1+r.Intn(4); j < np; j++ {
			lines = append(lines, genPattern(r))
		}
		eol := "\n"
		switch r.Intn(10) {
		case 0:
			eol = "\r\n"
		}
		c := strings.Join(lines, eol) + eol
		if r.Intn(15) == 0 {
			c = "\xef\xbb\xbf" + c
		}
		s.files[path.Join(d, ".gitignore")] = s.files[path.Join(d, ".gitignore")] + c
	}
	for i, np := 0, 3+r.Intn(10); i < np; i++ {
		depth := 1 + r.Intn(4)
		var segs []string
		for k := 0; k < depth; k++ {
			nm := names[r.Intn(len(names))]
			if icase && (nm == "FOO" || nm == "Foo") {
				nm = "foo"
			}
			segs = append(segs, nm)
		}
		p := strings.Join(segs, "/")
		isDir := r.Intn(3) == 0
		s.paths[p] = isDir
	}
	// fix up: prefixes must be dirs
	for p := range s.paths {
		parts := strings.Split(p, "/")
		for k := 1; k < len(parts); k++ {
			s.paths[strings.Join(parts[:k], "/")] = true
		}
	}
	// ensure dirs holding .gitignore exist
	for f := range s.files {
		d := path.Dir(f)
		if d != "." {
			parts := strings.Split(d, "/")
			for k := 1; k <= len(parts); k++ {
				s.paths[strings.Join(parts[:k], "/")] = true
			}
		}
	}
	return s
}

// parityCurated converts the curated cases into scenarios. A trailing "/"
// marks a directory. Paths starting with "/", containing "//", or with a "."
// or ".." segment are dropped: they cannot exist on disk inside the repo, and
// git check-ignore rejects them.
func parityCurated() []scenario {
	var out []scenario
	for i, c := range curatedCases() {
		s := scenario{name: fmt.Sprintf("curated-%d", i), icase: c.icase, files: c.files, paths: map[string]bool{}}
		for _, p := range c.paths {
			if strings.HasPrefix(p, "/") || strings.Contains(p, "//") || hasDotSegment(p) {
				continue
			}
			if dir, ok := strings.CutSuffix(p, "/"); ok {
				s.paths[dir] = true
			} else if !s.paths[p] {
				s.paths[p] = false
			}
		}
		addDirPrefixes(&s)
		out = append(out, s)
	}
	return out
}

// paritySeeded generates n scenarios from seed. It consumes the random source
// in the same order as the original audit tester, so seeds are comparable.
func paritySeeded(seed int64, n int, icase bool) []scenario {
	r := rand.New(rand.NewSource(seed))
	out := make([]scenario, 0, n)
	for i := range n {
		s := genScenario(r, icase)
		s.name = fmt.Sprintf("seed%d-%d", seed, i)
		s.icase = icase
		out = append(out, *s)
	}
	return out
}

// addDirPrefixes marks every proper prefix of a path, and every directory
// holding a .gitignore, as a directory.
func addDirPrefixes(s *scenario) {
	for p := range s.paths {
		parts := strings.Split(p, "/")
		for k := 1; k < len(parts); k++ {
			s.paths[strings.Join(parts[:k], "/")] = true
		}
	}
	for f := range s.files {
		d := path.Dir(f)
		if d == "." {
			continue
		}
		parts := strings.Split(d, "/")
		for k := 1; k <= len(parts); k++ {
			s.paths[strings.Join(parts[:k], "/")] = true
		}
	}
}

func hasDotSegment(p string) bool {
	for seg := range strings.SplitSeq(strings.TrimSuffix(p, "/"), "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
