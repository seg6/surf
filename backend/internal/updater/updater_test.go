package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"0.5.0", "0.5.0", 0},
		{"0.6.0", "0.5.9", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.4.9", "0.5.0", -1},
	} {
		got := CompareVersions(test.left, test.right)
		if got != test.want {
			t.Errorf("CompareVersions(%q, %q)=%d want %d", test.left, test.right, got, test.want)
		}
	}
}
