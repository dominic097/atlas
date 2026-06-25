package parser

import "testing"

// recvTypeSource exercises receiver-type capture: a type Foo with a method
// (f *Foo) Bar(), a caller that constructs &Foo{} and calls v.Bar(), plus a
// stdlib package call time.Now() that must NOT get a recv_type.
const recvTypeSource = `package sample

import "time"

type Foo struct {
	at time.Time
}

func (f *Foo) Bar() {}

func driver() {
	v := &Foo{}
	v.Bar()
	_ = time.Now()
}
`

func TestGoReceiverTypeCapture(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.go", "go", []byte(recvTypeSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// (a) The Bar method symbol must carry recv_type "Foo".
	var barFound bool
	for _, s := range res.Symbols {
		if s.Kind == "method" && s.Name == "Bar" {
			barFound = true
			recv, _ := s.Metadata["recv_type"].(string)
			if recv != "Foo" {
				t.Errorf("Bar method symbol recv_type = %q, want %q", recv, "Foo")
			}
		}
	}
	if !barFound {
		t.Fatalf("Bar method symbol not found; symbols=%+v", res.Symbols)
	}

	// (b) The v.Bar() call edge must carry recv_type "Foo".
	// (c) The time.Now() package call must have empty recv_type.
	var (
		barCallFound bool
		nowCallFound bool
	)
	for _, e := range res.Edges {
		if e.Kind != "calls" {
			continue
		}
		recv, _ := e.Metadata["recv_type"].(string)
		switch e.ToRef {
		case "Bar":
			barCallFound = true
			if recv != "Foo" {
				t.Errorf("v.Bar() call edge recv_type = %q, want %q (qualified_ref=%v)",
					recv, "Foo", e.Metadata["qualified_ref"])
			}
		case "Now":
			nowCallFound = true
			if recv != "" {
				t.Errorf("time.Now() call edge recv_type = %q, want empty (package call)", recv)
			}
		}
	}
	if !barCallFound {
		t.Errorf("v.Bar() call edge not found; edges=%+v", res.Edges)
	}
	if !nowCallFound {
		t.Errorf("time.Now() call edge not found; edges=%+v", res.Edges)
	}
}
