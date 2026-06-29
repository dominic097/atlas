// ignore.go implements a git-independent ignore mechanism for the index walk:
// an `.atlasignore` file (gitignore syntax) that works in ANY folder, including a
// plain documents directory that is not a git repo. It also inherits a `.gitignore`
// when the folder is not a git repo (in a real repo, `git ls-files` already honors
// .gitignore exactly — tracked-file-aware — so the matcher only reads .atlasignore
// there to avoid wrongly ignoring a tracked file that matches a pattern).
//
// The matcher is a self-contained gitignore-pattern engine (no dependency added):
// each pattern compiles to a regexp, evaluation is last-match-wins with `!`
// negation, `dir/` is directory-only, a leading `/` or an embedded `/` anchors to
// the ignore-file root, and a bare name matches at any depth. `*`/`?` stay within a
// path segment; `**` crosses segments.
package index

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	atlasIgnoreFile = ".atlasignore"
	gitIgnoreFile   = ".gitignore"
)

type ignoreRule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

type ignoreMatcher struct {
	rules []ignoreRule
}

// loadIgnoreMatcher reads root/.atlasignore (always) and, when alsoGitignore is
// true, root/.gitignore, returning nil when neither yields a rule so callers can
// skip matching entirely. Order is preserved (.atlasignore first, then .gitignore)
// so a later negation can override an earlier ignore across both files.
func loadIgnoreMatcher(root string, alsoGitignore bool) *ignoreMatcher {
	var rules []ignoreRule
	rules = append(rules, readIgnoreFile(filepath.Join(root, atlasIgnoreFile))...)
	if alsoGitignore {
		rules = append(rules, readIgnoreFile(filepath.Join(root, gitIgnoreFile))...)
	}
	if len(rules) == 0 {
		return nil
	}
	return &ignoreMatcher{rules: rules}
}

func readIgnoreFile(path string) []ignoreRule {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rules []ignoreRule
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if r, ok := compileIgnorePattern(sc.Text()); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// ignored reports whether rel (a slash-separated path relative to the ignore-file
// root) is ignored. isDir gates directory-only (`dir/`) rules. Last matching rule
// wins, so a trailing `!pattern` re-includes a path an earlier rule excluded.
func (m *ignoreMatcher) ignored(rel string, isDir bool) bool {
	if m == nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	out := false
	for i := range m.rules {
		r := &m.rules[i]
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(rel) {
			out = !r.negate
		}
	}
	return out
}

// compileIgnorePattern turns one gitignore line into a rule. It returns ok=false
// for blank lines and comments.
func compileIgnorePattern(line string) (ignoreRule, bool) {
	// A leading '#' is a comment unless escaped ("\#"); trailing unescaped spaces
	// are stripped (gitignore rules).
	raw := strings.TrimRight(line, " \t")
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ignoreRule{}, false
	}
	p := raw
	negate := false
	if strings.HasPrefix(p, "!") {
		negate = true
		p = p[1:]
	}
	p = strings.TrimPrefix(p, `\`) // unescape a leading \# or \!
	dirOnly := false
	if strings.HasSuffix(p, "/") {
		dirOnly = true
		p = strings.TrimSuffix(p, "/")
	}
	if p == "" {
		return ignoreRule{}, false
	}
	// A slash anywhere but the (already-stripped) trailing one anchors the pattern
	// to the root; otherwise it matches a basename at any depth.
	anchored := strings.HasPrefix(p, "/") || strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")

	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	b.WriteString(globToRegex(p))
	// Match the path itself OR anything beneath it, so a matched directory also
	// covers its descendants (belt-and-suspenders with the walk's SkipDir).
	b.WriteString("(?:/.*)?$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return ignoreRule{}, false
	}
	return ignoreRule{re: re, negate: negate, dirOnly: dirOnly}, true
}

// globToRegex converts gitignore glob syntax to a regexp body: `**` crosses path
// separators, `*` and `?` stay within one segment, and regex metacharacters are
// escaped.
func globToRegex(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				i++ // consume second '*'
				if i+1 < len(p) && p[i+1] == '/' {
					i++ // consume the slash after '**/'
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
