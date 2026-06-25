package coverage

import "testing"

// byFile is a test helper turning the ordered slice into a lookup map.
func byFile(files []FileCoverage) map[string]map[int]bool {
	m := make(map[string]map[int]bool, len(files))
	for _, f := range files {
		m[f.File] = f.Covered
	}
	return m
}

func TestParseGoCoverprofile(t *testing.T) {
	const profile = "mode: set\n" +
		"foo/bar.go:10.2,12.3 2 1\n" +
		"foo/bar.go:20.2,20.10 1 0\n"

	format, files, err := Parse([]byte(profile))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "go" {
		t.Fatalf("format = %q, want %q", format, "go")
	}

	cov := byFile(files)["foo/bar.go"]
	if cov == nil {
		t.Fatalf("no coverage recorded for foo/bar.go; got files %+v", files)
	}

	// Lines 10, 11, 12 are in a block with count > 0 -> covered.
	for _, ln := range []int{10, 11, 12} {
		if !cov[ln] {
			t.Errorf("line %d: covered = false, want true", ln)
		}
	}
	// Line 20 is in a block with count == 0 -> NOT covered.
	if cov[20] {
		t.Errorf("line 20: covered = true, want false")
	}
	// Sanity: a line never mentioned should not be covered.
	if cov[15] {
		t.Errorf("line 15: covered = true, want false (not in any block)")
	}
}

func TestParseLCOV(t *testing.T) {
	const lcov = "SF:foo/bar.go\n" +
		"DA:5,3\n" +
		"DA:6,0\n" +
		"end_of_record\n"

	format, files, err := Parse([]byte(lcov))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "lcov" {
		t.Fatalf("format = %q, want %q", format, "lcov")
	}

	cov := byFile(files)["foo/bar.go"]
	if cov == nil {
		t.Fatalf("no coverage recorded for foo/bar.go; got files %+v", files)
	}

	// DA:5,3 -> count > 0 -> covered.
	if !cov[5] {
		t.Errorf("line 5: covered = false, want true")
	}
	// DA:6,0 -> count == 0 -> NOT covered.
	if cov[6] {
		t.Errorf("line 6: covered = true, want false")
	}
}

func TestParseLCOVWithChecksum(t *testing.T) {
	// DA lines may carry a trailing checksum: DA:<line>,<count>,<hash>.
	const lcov = "TN:\n" +
		"SF:pkg/util.go\n" +
		"DA:1,4,abc123\n" +
		"DA:2,0,def456\n" +
		"end_of_record\n"

	format, files, err := Parse([]byte(lcov))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "lcov" {
		t.Fatalf("format = %q, want lcov", format)
	}
	cov := byFile(files)["pkg/util.go"]
	if !cov[1] {
		t.Errorf("line 1: covered = false, want true")
	}
	if cov[2] {
		t.Errorf("line 2: covered = true, want false")
	}
}

func TestParseGoCoverageUnion(t *testing.T) {
	// A line covered in one block stays covered even if a later block reports
	// the (overlapping) region as not covered.
	const profile = "mode: count\n" +
		"a/b.go:1.0,3.0 1 0\n" +
		"a/b.go:2.0,2.5 1 7\n"

	_, files, err := Parse([]byte(profile))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	cov := byFile(files)["a/b.go"]
	if !cov[2] {
		t.Errorf("line 2: covered = false, want true (union of overlapping blocks)")
	}
	if cov[1] || cov[3] {
		t.Errorf("lines 1/3 should be not covered: got %v / %v", cov[1], cov[3])
	}
}

func TestParseDeterministicOrder(t *testing.T) {
	const profile = "mode: set\n" +
		"z/last.go:1.0,1.0 1 1\n" +
		"a/first.go:1.0,1.0 1 1\n"
	_, files, err := Parse([]byte(profile))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	// First-seen order is preserved (NOT sorted): z/last.go appears first.
	if files[0].File != "z/last.go" || files[1].File != "a/first.go" {
		t.Errorf("order = [%s, %s], want [z/last.go, a/first.go]", files[0].File, files[1].File)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t\n"},
		{"unknown", "this is not coverage data\nrandom text\n"},
		{"json", "{\"hello\": \"world\"}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Parse([]byte(tc.content)); err == nil {
				t.Errorf("Parse(%q) = nil error, want error", tc.content)
			}
		})
	}
}
