package hook

import "testing"

func TestStripLayoutPanes(t *testing.T) {
	cases := []struct {
		in   string
		drop []int
		want string // body only; checksum is checked separately
	}{
		// sidebar left of a single content pane
		{"1416,257x71,0,0{25x71,0,0,1,231x71,26,0,0}", []int{1}, "257x71,0,0,0"},
		// sidebar left of a vertical split of two content panes
		{"bbe8,257x71,0,0{25x71,0,0,7,231x71,26,0[231x35,26,0,6,231x35,26,36,8]}", []int{7},
			"257x71,0,0[257x35,0,0,6,257x35,0,36,8]"},
		// nothing to drop leaves the body unchanged
		{"1416,257x71,0,0{25x71,0,0,1,231x71,26,0,0}", []int{99}, "257x71,0,0{25x71,0,0,1,231x71,26,0,0}"},
	}
	for _, tc := range cases {
		got, ok := stripLayoutPanes(tc.in, tc.drop)
		if !ok {
			t.Fatalf("%s: not ok", tc.in)
		}
		if got[5:] != tc.want {
			t.Errorf("%s: got %q want body %q", tc.in, got, tc.want)
		}
		if got[:4] != fmtChecksum(got[5:]) {
			t.Errorf("%s: checksum %s != %s", tc.in, got[:4], fmtChecksum(got[5:]))
		}
	}
	// round trip: the original string's checksum must match tmux's
	if fmtChecksum("257x71,0,0{25x71,0,0,1,231x71,26,0,0}") != "1416" {
		t.Errorf("checksum algorithm disagrees with tmux: %s", fmtChecksum("257x71,0,0{25x71,0,0,1,231x71,26,0,0}"))
	}
	if _, ok := stripLayoutPanes("1416,257x71,0,0,1", []int{1}); ok {
		t.Errorf("dropping the only pane must report not ok")
	}
}

func fmtChecksum(body string) string {
	return string([]byte{hex(layoutChecksum(body) >> 12), hex(layoutChecksum(body) >> 8), hex(layoutChecksum(body) >> 4), hex(layoutChecksum(body))})
}

func hex(v uint16) byte { return "0123456789abcdef"[v&0xf] }
