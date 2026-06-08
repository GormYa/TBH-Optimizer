package internal

import "testing"

func TestParseVer(t *testing.T) {
	cases := map[string][3]int{
		"1.2.3":   {1, 2, 3},
		"v1.2.3":  {1, 2, 3},
		" 1.0 ":   {1, 0, 0},
		"2":       {2, 0, 0},
		"1.10.0":  {1, 10, 0},
		"garbage": {0, 0, 0},
	}
	for in, want := range cases {
		if got := parseVer(in); got != want {
			t.Errorf("parseVer(%q) = %v, quero %v", in, got, want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		remote, local string
		want          bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},
		{"v1.2.0", "1.1.5", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"1.0", "1.0.0", false},
		{"1.0.0", "1.0", false},
	}
	for _, c := range cases {
		if got := isNewer(c.remote, c.local); got != c.want {
			t.Errorf("isNewer(%q,%q) = %v, quero %v", c.remote, c.local, got, c.want)
		}
	}
}
