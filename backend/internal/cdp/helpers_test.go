package cdp

import "testing"

func TestNavigationHistoryDirection(t *testing.T) {
	cases := []struct {
		name     string
		history  NavigationHistory
		wantBack bool
		wantFwd  bool
	}{
		{name: "first", history: NavigationHistory{CurrentIndex: 0, Entries: []NavigationEntry{{ID: 1}, {ID: 2}}}, wantBack: false, wantFwd: true},
		{name: "middle", history: NavigationHistory{CurrentIndex: 1, Entries: []NavigationEntry{{ID: 1}, {ID: 2}, {ID: 3}}}, wantBack: true, wantFwd: true},
		{name: "last", history: NavigationHistory{CurrentIndex: 1, Entries: []NavigationEntry{{ID: 1}, {ID: 2}}}, wantBack: true, wantFwd: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.history.CanGoBack(); got != tc.wantBack {
				t.Fatalf("CanGoBack() = %t, want %t", got, tc.wantBack)
			}
			if got := tc.history.CanGoForward(); got != tc.wantFwd {
				t.Fatalf("CanGoForward() = %t, want %t", got, tc.wantFwd)
			}
		})
	}
}
