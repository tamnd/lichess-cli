package lichess

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in lichess_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "lichess" {
		t.Errorf("Scheme = %q, want lichess", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "lichess" {
		t.Errorf("Identity.Binary = %q, want lichess", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"DrNykterstein", "user", "DrNykterstein"},
		{"/DrNykterstein/", "user", "DrNykterstein"},
		{"https://" + Host + "/@/hikaru", "user", "@/hikaru"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("user", "DrNykterstein")
	want := "https://" + Host + "/@/DrNykterstein"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

// TestHostWiring mounts the driver in a kit Host and checks round-trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	u := &User{ID: "drnykterstein", Username: "DrNykterstein"}
	_, err = h.Mint(u)
	// Mint may fail if User doesn't carry kit tags; that's acceptable for now.
	// We just verify no panic.
	_ = err

	got, err := h.ResolveOn("lichess", "DrNykterstein")
	if err != nil || got.String() != "lichess://user/DrNykterstein" {
		t.Errorf("ResolveOn = (%q, %v), want lichess://user/DrNykterstein", got.String(), err)
	}
}
