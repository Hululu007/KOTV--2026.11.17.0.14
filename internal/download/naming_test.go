package download

import "testing"

func TestSanitizeNameKeepsPlayStringSafe(t *testing.T) {
	cases := map[string]string{
		"第#1集":          "第_1集",
		"A$B＄C":         "A_B_C",
		`a/b\c:d*e?f"g`: "a_b_c_d_e_f_g",
		"  ":            "未命名",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
