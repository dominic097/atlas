// Package coverage parses runtime test-coverage profiles into per-file sets of
// covered line numbers. It is pure: it depends only on the stdlib and has no
// knowledge of the store, graph, or engine. Callers map the covered lines back
// onto symbols to report RUNTIME coverage (as opposed to static call-graph
// reachability).
//
// Four input formats are auto-detected and supported:
//
//   - Go coverprofile (`go test -coverprofile`): a leading "mode:" line followed
//     by data lines of the form
//     "path/file.go:startLine.startCol,endLine.endCol numStmt count".
//     When count > 0 every line in [startLine, endLine] is marked covered.
//   - LCOV: records delimited by "end_of_record", each opened by "SF:<file>"
//     and carrying "DA:<line>,<count>" entries. count > 0 marks the line
//     covered.
//   - Cobertura XML (also emitted by coverage.py): a <coverage> root whose
//     <class filename=..> elements carry <line number=.. hits=..> entries.
//     hits > 0 marks the line covered.
//   - JaCoCo XML: a <report> root whose <sourcefile name=..> elements (nested
//     under <package name=..>) carry <line nr=.. ci=..> entries. ci (covered
//     instructions) > 0 marks the line covered.
package coverage

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// FileCoverage holds the set of covered line numbers for a single file. Covered
// maps a 1-based line number to true when at least one execution counter for
// that line was positive. Lines that appear in the profile but were never
// executed are intentionally absent (not present with a false value), except
// where a parser explicitly records a not-covered line — callers should treat
// "absent or false" as not covered.
type FileCoverage struct {
	File    string
	Covered map[int]bool
}

// Parse auto-detects the coverage format of content and returns the detected
// format ("go", "lcov", "cobertura", or "jacoco") together with the per-file
// covered line sets. Files are returned in first-seen order so output is
// deterministic for a given input. Content that matches no format yields an
// error.
func Parse(content []byte) (format string, files []FileCoverage, err error) {
	trimmed := bytes.TrimLeft(content, " \t\r\n")
	if len(trimmed) == 0 {
		return "", nil, fmt.Errorf("coverage: empty content")
	}

	// XML-based formats (Cobertura / coverage.py, JaCoCo) start with an XML
	// declaration or a bare root element.
	if trimmed[0] == '<' {
		switch xmlFormat(trimmed) {
		case "jacoco":
			files, err = parseJacoco(content)
			return "jacoco", files, err
		case "cobertura":
			files, err = parseCobertura(content)
			return "cobertura", files, err
		}
	}

	switch {
	case bytes.HasPrefix(trimmed, []byte("mode:")):
		files, err = parseGo(content)
		return "go", files, err
	case looksLikeLCOV(trimmed):
		files, err = parseLCOV(content)
		return "lcov", files, err
	default:
		return "", nil, fmt.Errorf("coverage: unrecognized format (expected Go coverprofile 'mode:' header, LCOV records, or Cobertura/JaCoCo XML)")
	}
}

// xmlFormat sniffs XML content by its actual ROOT element rather than a
// substring scan (which would misfire on a tag name appearing inside a comment
// or attribute value): a <report> root is JaCoCo, a <coverage> root is Cobertura
// (and the schema-compatible coverage.py output). Anything else returns "".
func xmlFormat(trimmed []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		// The first StartElement is the document root; the XML declaration, any
		// comments, and the DOCTYPE are non-StartElement tokens we skip over.
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "report":
				return "jacoco"
			case "coverage":
				return "cobertura"
			default:
				return ""
			}
		}
	}
}

// looksLikeLCOV reports whether the (left-trimmed) content begins with a token
// recognizable as an LCOV record line. LCOV files commonly start with TN: (test
// name) or SF: (source file), and may carry DA:/FN:/etc. lines.
func looksLikeLCOV(trimmed []byte) bool {
	for _, prefix := range [][]byte{
		[]byte("SF:"), []byte("TN:"), []byte("DA:"),
		[]byte("FN:"), []byte("FNDA:"), []byte("BRDA:"),
	} {
		if bytes.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// fileSet accumulates covered lines per file while preserving first-seen order.
type fileSet struct {
	order  []string
	byFile map[string]map[int]bool
}

func newFileSet() *fileSet {
	return &fileSet{byFile: make(map[string]map[int]bool)}
}

// lines returns the (creating if necessary) covered-line map for file.
func (fs *fileSet) lines(file string) map[int]bool {
	m, ok := fs.byFile[file]
	if !ok {
		m = make(map[int]bool)
		fs.byFile[file] = m
		fs.order = append(fs.order, file)
	}
	return m
}

// mark records line for file with the given covered state. Once a line is
// covered it stays covered even if a later record reports it as not covered
// (coverage is a union across records/test runs).
func (fs *fileSet) mark(file string, line int, covered bool) {
	if line <= 0 {
		return
	}
	m := fs.lines(file)
	if covered {
		m[line] = true
	} else if _, exists := m[line]; !exists {
		m[line] = false
	}
}

// result materializes the accumulated coverage into the ordered slice form.
func (fs *fileSet) result() []FileCoverage {
	out := make([]FileCoverage, 0, len(fs.order))
	for _, f := range fs.order {
		out = append(out, FileCoverage{File: f, Covered: fs.byFile[f]})
	}
	return out
}

// parseGo parses a Go coverprofile. The first non-empty line is the mode
// header; subsequent lines are coverage blocks. A block reads:
//
//	name.go:startLine.startCol,endLine.endCol numStmt count
//
// When count > 0, every line in [startLine, endLine] is marked covered.
func parseGo(content []byte) ([]FileCoverage, error) {
	fs := newFileSet()
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	seenMode := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !seenMode {
			if !strings.HasPrefix(line, "mode:") {
				return nil, fmt.Errorf("coverage: go coverprofile missing 'mode:' header")
			}
			seenMode = true
			continue
		}
		file, start, end, count, err := parseGoBlock(line)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			for ln := start; ln <= end; ln++ {
				fs.mark(file, ln, true)
			}
		} else {
			for ln := start; ln <= end; ln++ {
				fs.mark(file, ln, false)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("coverage: read go coverprofile: %w", err)
	}
	if !seenMode {
		return nil, fmt.Errorf("coverage: go coverprofile missing 'mode:' header")
	}
	return fs.result(), nil
}

// parseGoBlock splits one coverprofile data line into its components.
// Input shape: "path/file.go:startLine.startCol,endLine.endCol numStmt count".
func parseGoBlock(line string) (file string, startLine, endLine, count int, err error) {
	// The file path may itself contain ':' (e.g. Windows volume) but the
	// coverprofile position block is the final ":<pos> <stmt> <count>" portion.
	// Split off the trailing " numStmt count" first.
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", 0, 0, 0, fmt.Errorf("coverage: malformed go coverage line %q", line)
	}
	count, err = strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("coverage: bad count in %q: %w", line, err)
	}
	// fields[len-2] is numStmt (validated as int but otherwise unused).
	if _, err = strconv.Atoi(fields[len(fields)-2]); err != nil {
		return "", 0, 0, 0, fmt.Errorf("coverage: bad stmt count in %q: %w", line, err)
	}
	// Everything before the last two fields is "file:positions" (positions has
	// no spaces, so it is the last whitespace-delimited token of that prefix).
	prefix := strings.TrimSpace(line[:strings.LastIndex(line, fields[len(fields)-2])])
	colon := strings.LastIndex(prefix, ":")
	if colon < 0 {
		return "", 0, 0, 0, fmt.Errorf("coverage: missing ':' position separator in %q", line)
	}
	file = prefix[:colon]
	positions := prefix[colon+1:]
	startLine, endLine, err = parseGoPositions(positions)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("coverage: %w in %q", err, line)
	}
	if file == "" {
		return "", 0, 0, 0, fmt.Errorf("coverage: empty file path in %q", line)
	}
	return file, startLine, endLine, count, nil
}

// parseGoPositions parses "startLine.startCol,endLine.endCol" -> start/end line.
func parseGoPositions(pos string) (startLine, endLine int, err error) {
	comma := strings.IndexByte(pos, ',')
	if comma < 0 {
		return 0, 0, fmt.Errorf("missing ',' in position block %q", pos)
	}
	startLine, err = lineFromPos(pos[:comma])
	if err != nil {
		return 0, 0, err
	}
	endLine, err = lineFromPos(pos[comma+1:])
	if err != nil {
		return 0, 0, err
	}
	if endLine < startLine {
		return 0, 0, fmt.Errorf("end line %d before start line %d", endLine, startLine)
	}
	return startLine, endLine, nil
}

// lineFromPos parses "line.col" (or a bare "line") and returns the line number.
func lineFromPos(p string) (int, error) {
	p = strings.TrimSpace(p)
	if dot := strings.IndexByte(p, '.'); dot >= 0 {
		p = p[:dot]
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("bad line number %q", p)
	}
	return n, nil
}

// parseLCOV parses LCOV tracefile content. Lines of interest:
//
//	SF:<source file>     -> opens a record
//	DA:<line>,<count>    -> count > 0 marks the line covered
//	end_of_record        -> closes the current record
//
// All other line types (TN:, FN:, FNDA:, BRDA:, LF:, LH:, ...) are ignored.
func parseLCOV(content []byte) ([]FileCoverage, error) {
	fs := newFileSet()
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	current := ""
	sawSF := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "SF:"):
			current = strings.TrimSpace(line[len("SF:"):])
			if current != "" {
				sawSF = true
				fs.lines(current) // register the file even if it has no DA lines
			}
		case strings.HasPrefix(line, "DA:"):
			if current == "" {
				continue
			}
			ln, count, err := parseLCOVData(line)
			if err != nil {
				return nil, err
			}
			fs.mark(current, ln, count > 0)
		case line == "end_of_record":
			current = ""
		default:
			// ignore TN:, FN:, FNDA:, BRDA:, LF:, LH:, FNF:, FNH:, BRF:, BRH:, etc.
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("coverage: read lcov: %w", err)
	}
	if !sawSF {
		return nil, fmt.Errorf("coverage: lcov input contained no SF: record")
	}
	return fs.result(), nil
}

// parseLCOVData parses "DA:<line>,<count>" -> line, count.
func parseLCOVData(line string) (lineNo, count int, err error) {
	body := strings.TrimSpace(line[len("DA:"):])
	comma := strings.IndexByte(body, ',')
	if comma < 0 {
		return 0, 0, fmt.Errorf("coverage: malformed DA line %q", line)
	}
	lineNo, err = strconv.Atoi(strings.TrimSpace(body[:comma]))
	if err != nil {
		return 0, 0, fmt.Errorf("coverage: bad line number in %q: %w", line, err)
	}
	// LCOV count may carry a checksum after a second comma (DA:line,count,hash).
	countStr := body[comma+1:]
	if c2 := strings.IndexByte(countStr, ','); c2 >= 0 {
		countStr = countStr[:c2]
	}
	count, err = strconv.Atoi(strings.TrimSpace(countStr))
	if err != nil {
		return 0, 0, fmt.Errorf("coverage: bad count in %q: %w", line, err)
	}
	return lineNo, count, nil
}

// coberturaReport models the subset of the Cobertura XML schema (also emitted by
// coverage.py) that we need: per-class filenames and their per-line hit counts.
type coberturaReport struct {
	XMLName xml.Name         `xml:"coverage"`
	Classes []coberturaClass `xml:"packages>package>classes>class"`
}

type coberturaClass struct {
	Filename string          `xml:"filename,attr"`
	Lines    []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number int `xml:"number,attr"`
	Hits   int `xml:"hits,attr"`
}

// parseCobertura parses Cobertura/coverage.py XML. For each <class> the filename
// attribute is the file and each <line number=.. hits=..> with hits > 0 marks
// that line covered. Multiple <class> blocks sharing a filename are unioned.
func parseCobertura(content []byte) ([]FileCoverage, error) {
	var report coberturaReport
	if err := xml.Unmarshal(content, &report); err != nil {
		return nil, fmt.Errorf("coverage: parse cobertura xml: %w", err)
	}
	fs := newFileSet()
	for _, class := range report.Classes {
		file := strings.TrimSpace(class.Filename)
		if file == "" {
			continue
		}
		fs.lines(file) // register the file even if it has no line entries
		for _, ln := range class.Lines {
			fs.mark(file, ln.Number, ln.Hits > 0)
		}
	}
	if len(fs.order) == 0 {
		return nil, fmt.Errorf("coverage: cobertura xml contained no <class filename=..> entries")
	}
	return fs.result(), nil
}

// jacocoReport models the subset of the JaCoCo XML schema that we need: per
// package name plus its sourcefiles, each carrying per-line covered-instruction
// counts. We use <sourcefile> (reliable per-line) rather than <class>.
type jacocoReport struct {
	XMLName  xml.Name        `xml:"report"`
	Packages []jacocoPackage `xml:"package"`
}

type jacocoPackage struct {
	Name        string             `xml:"name,attr"`
	Sourcefiles []jacocoSourcefile `xml:"sourcefile"`
}

type jacocoSourcefile struct {
	Name  string       `xml:"name,attr"`
	Lines []jacocoLine `xml:"line"`
}

type jacocoLine struct {
	Nr int `xml:"nr,attr"`
	Ci int `xml:"ci,attr"`
}

// parseJacoco parses JaCoCo XML. The file path is the package name joined to the
// sourcefile name (e.g. "com/foo" + "Bar.java" -> "com/foo/Bar.java"). A line is
// covered when its ci (covered instructions) count is > 0. Sourcefiles sharing a
// resolved path are unioned.
func parseJacoco(content []byte) ([]FileCoverage, error) {
	var report jacocoReport
	if err := xml.Unmarshal(content, &report); err != nil {
		return nil, fmt.Errorf("coverage: parse jacoco xml: %w", err)
	}
	fs := newFileSet()
	for _, pkg := range report.Packages {
		pkgName := strings.Trim(strings.TrimSpace(pkg.Name), "/")
		for _, sf := range pkg.Sourcefiles {
			name := strings.TrimSpace(sf.Name)
			if name == "" {
				continue
			}
			file := name
			if pkgName != "" {
				file = pkgName + "/" + name
			}
			fs.lines(file) // register the file even if it has no line entries
			for _, ln := range sf.Lines {
				fs.mark(file, ln.Nr, ln.Ci > 0)
			}
		}
	}
	if len(fs.order) == 0 {
		return nil, fmt.Errorf("coverage: jacoco xml contained no <sourcefile name=..> entries")
	}
	return fs.result(), nil
}
