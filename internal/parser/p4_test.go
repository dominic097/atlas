package parser

import "testing"

// TestP4Symbols covers P4_16 top-level declaration extraction (github.com/p4lang):
// parser/control/package, action/table, header/header_union/struct, enum/extern,
// typedef-type, const, and parser states.
func TestP4Symbols(t *testing.T) {
	src := []byte(`
typedef bit<9>  egressSpec_t;
type bit<32> ip4Addr_t;

header ethernet_t { macAddr_t dstAddr; }
header_union ip_t { ipv4_t v4; ipv6_t v6; }
struct headers { ethernet_t ethernet; }

const bit<16> TYPE_IPV4 = 0x800;
enum bit<8> FieldLists { f1, f2 }
extern Checksum16 { void clear(); }

parser MyParser(packet_in packet, out headers hdr) {
    state start { transition parse_eth; }
    state parse_eth { transition accept; }
}

control MyIngress(inout headers hdr) {
    action drop() { mark_to_drop(); }
    table mac_lookup { key = {} actions = { drop; } }
    apply { mac_lookup.apply(); }
}

package V1Switch(MyParser p, MyIngress ig);
`)
	res, err := Parse("repo", "owner/repo", "router.p4", "p4", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := map[string]string{}
	for _, s := range res.Symbols {
		got[s.Name] = s.Kind
	}
	want := map[string]string{
		"egressSpec_t": "type",
		"ip4Addr_t":    "type",
		"ethernet_t":   "header",
		"ip_t":         "header_union",
		"headers":      "struct",
		"TYPE_IPV4":    "constant",
		"FieldLists":   "enum",
		"Checksum16":   "extern",
		"MyParser":     "parser",
		"start":        "state",
		"parse_eth":    "state",
		"MyIngress":    "control",
		"drop":         "action",
		"mac_lookup":   "table",
		"V1Switch":     "package",
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("P4 symbol %q: got kind %q, want %q", name, got[name], kind)
		}
	}
	// Precision: `header X {` must not also fire the header_union rule (and vice
	// versa), so each construct yields exactly its own kind — no spurious dup kinds.
	if got["ethernet_t"] == "header_union" || got["ip_t"] == "header" {
		t.Errorf("header / header_union rules collided: %v", got)
	}
}
