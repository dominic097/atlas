package cli

import (
	"testing"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func TestApplyIndexDefaultsUsesGlobalRepo(t *testing.T) {
	old := gf
	t.Cleanup(func() { gf = old })
	gf.repo = "MsysTechnologiesllc/Aziron"

	in := atlas.IndexInput{}
	applyIndexDefaults(&in)
	if in.Repo != "MsysTechnologiesllc/Aziron" {
		t.Fatalf("Repo = %q, want global repo", in.Repo)
	}

	in.Repo = "local/repo"
	applyIndexDefaults(&in)
	if in.Repo != "local/repo" {
		t.Fatalf("Repo was overwritten: %q", in.Repo)
	}
}
