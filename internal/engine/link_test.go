package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveRepoRef(t *testing.T) {
	// A real existing directory: full_name = basename, root = abs path.
	dir := t.TempDir()
	wantBase := filepath.Base(dir)
	if fn, root := deriveRepoRef(dir); fn != wantBase || root != dir {
		t.Errorf("deriveRepoRef(%q) = (%q, %q), want (%q, %q)", dir, fn, root, wantBase, dir)
	}

	cases := []struct {
		name     string
		ref      string
		wantName string
		wantRoot string
	}{
		{
			name:     "ssh scp-like remote",
			ref:      "git@github.com:Acme/Web.git",
			wantName: "Acme/Web",
			wantRoot: "git@github.com:Acme/Web.git",
		},
		{
			name:     "ssh scp-like remote no .git",
			ref:      "git@github.com:Acme/Web",
			wantName: "Acme/Web",
			wantRoot: "git@github.com:Acme/Web",
		},
		{
			name:     "https remote with .git",
			ref:      "https://github.com/Acme/Web.git",
			wantName: "Acme/Web",
			wantRoot: "https://github.com/Acme/Web.git",
		},
		{
			name:     "https remote without .git",
			ref:      "https://gitlab.example.com/Acme/Web",
			wantName: "Acme/Web",
			wantRoot: "https://gitlab.example.com/Acme/Web",
		},
		{
			name:     "ssh url scheme",
			ref:      "ssh://git@github.com/Acme/Web.git",
			wantName: "Acme/Web",
			wantRoot: "ssh://git@github.com/Acme/Web.git",
		},
		{
			name:     "bare org/name is verbatim, not a path",
			ref:      "Acme/Web",
			wantName: "Acme/Web",
			wantRoot: "Acme/Web",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, root := deriveRepoRef(tc.ref)
			if fn != tc.wantName || root != tc.wantRoot {
				t.Errorf("deriveRepoRef(%q) = (%q, %q), want (%q, %q)", tc.ref, fn, root, tc.wantName, tc.wantRoot)
			}
		})
	}
}

func TestDeriveRepoRefPathLooking(t *testing.T) {
	// A path-looking ref that does NOT exist on disk is still treated as a path
	// (absolute and relative "./"/"../" shapes), full_name = basename of its abs.
	abs := filepath.Join(os.TempDir(), "atlas-nonexistent-xyz", "MyRepo")
	fn, root := deriveRepoRef(abs)
	if fn != "MyRepo" {
		t.Errorf("deriveRepoRef(%q) full_name = %q, want MyRepo", abs, fn)
	}
	if root != abs {
		t.Errorf("deriveRepoRef(%q) root = %q, want %q", abs, root, abs)
	}

	rel := "./some/local/Path"
	wantAbs, _ := filepath.Abs(rel)
	fn, root = deriveRepoRef(rel)
	if fn != "Path" {
		t.Errorf("deriveRepoRef(%q) full_name = %q, want Path", rel, fn)
	}
	if root != wantAbs {
		t.Errorf("deriveRepoRef(%q) root = %q, want %q", rel, root, wantAbs)
	}
}
