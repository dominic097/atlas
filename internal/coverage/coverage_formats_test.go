package coverage

import "testing"

func TestParseCobertura(t *testing.T) {
	// Minimal Cobertura document (the same schema coverage.py emits): two
	// classes, one of which repeats the filename to exercise the union.
	const doc = `<?xml version="1.0" ?>
<coverage version="1.0">
  <packages>
    <package name="pkg">
      <classes>
        <class name="Foo" filename="src/foo.py">
          <lines>
            <line number="1" hits="3"/>
            <line number="2" hits="0"/>
            <line number="4" hits="1"/>
          </lines>
        </class>
        <class name="FooAgain" filename="src/foo.py">
          <lines>
            <line number="2" hits="5"/>
          </lines>
        </class>
        <class name="Bar" filename="src/bar.py">
          <lines>
            <line number="10" hits="0"/>
          </lines>
        </class>
      </classes>
    </package>
  </packages>
</coverage>`

	format, files, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "cobertura" {
		t.Fatalf("format = %q, want %q", format, "cobertura")
	}

	idx := byFile(files)
	foo := idx["src/foo.py"]
	if foo == nil {
		t.Fatalf("no coverage recorded for src/foo.py; got files %+v", files)
	}
	// hits > 0 -> covered.
	if !foo[1] {
		t.Errorf("foo line 1: covered = false, want true")
	}
	if !foo[4] {
		t.Errorf("foo line 4: covered = false, want true")
	}
	// Line 2 is hits=0 in the first class but hits=5 in the second; union -> covered.
	if !foo[2] {
		t.Errorf("foo line 2: covered = false, want true (union across classes)")
	}
	// A line never mentioned is not covered.
	if foo[3] {
		t.Errorf("foo line 3: covered = true, want false")
	}

	bar := idx["src/bar.py"]
	if bar == nil {
		t.Fatalf("no coverage recorded for src/bar.py")
	}
	// hits=0 -> not covered.
	if bar[10] {
		t.Errorf("bar line 10: covered = true, want false")
	}
}

func TestParseJacoco(t *testing.T) {
	// Minimal JaCoCo document. File path = package-name + "/" + sourcefile-name.
	// ci (covered instructions) > 0 marks a line covered. A second sourcefile in
	// a second package, plus a repeated sourcefile, exercises join + union.
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="demo">
  <package name="com/foo">
    <sourcefile name="Bar.java">
      <line nr="5" mi="0" ci="4"/>
      <line nr="6" mi="2" ci="0"/>
      <line nr="9" mi="0" ci="1"/>
    </sourcefile>
    <sourcefile name="Bar.java">
      <line nr="6" mi="0" ci="3"/>
    </sourcefile>
  </package>
  <package name="com/baz">
    <sourcefile name="Qux.java">
      <line nr="1" mi="3" ci="0"/>
    </sourcefile>
  </package>
</report>`

	format, files, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "jacoco" {
		t.Fatalf("format = %q, want %q", format, "jacoco")
	}

	idx := byFile(files)
	bar := idx["com/foo/Bar.java"]
	if bar == nil {
		t.Fatalf("no coverage recorded for com/foo/Bar.java; got files %+v", files)
	}
	// ci > 0 -> covered.
	if !bar[5] {
		t.Errorf("Bar line 5: covered = false, want true")
	}
	if !bar[9] {
		t.Errorf("Bar line 9: covered = false, want true")
	}
	// Line 6 is ci=0 in the first sourcefile but ci=3 in the second; union -> covered.
	if !bar[6] {
		t.Errorf("Bar line 6: covered = false, want true (union across sourcefiles)")
	}
	// A line never mentioned is not covered.
	if bar[7] {
		t.Errorf("Bar line 7: covered = true, want false")
	}

	qux := idx["com/baz/Qux.java"]
	if qux == nil {
		t.Fatalf("no coverage recorded for com/baz/Qux.java")
	}
	// ci=0 -> not covered.
	if qux[1] {
		t.Errorf("Qux line 1: covered = true, want false")
	}
}

func TestParseJacocoNoDoctype(t *testing.T) {
	// JaCoCo without a DOCTYPE must still be detected via its <report> root.
	const doc = `<report name="demo">
  <package name="com/foo">
    <sourcefile name="Baz.java">
      <line nr="1" mi="0" ci="2"/>
    </sourcefile>
  </package>
</report>`

	format, files, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if format != "jacoco" {
		t.Fatalf("format = %q, want %q", format, "jacoco")
	}
	if !byFile(files)["com/foo/Baz.java"][1] {
		t.Errorf("Baz line 1: covered = false, want true")
	}
}
