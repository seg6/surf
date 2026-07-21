package browser

import "testing"

func TestHistoryTitlePatchesMatchingURL(t *testing.T) {
	s := NewStore(t.TempDir())
	s.AddHistory("https://example.com/one", "")
	s.SetTitle("https://example.com/one", "One")
	s.AddHistory("https://example.com/two", "")
	s.SetTitle("https://example.com/two", "Two")

	recent := s.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("Recent len = %d, want 2", len(recent))
	}
	if recent[0].URL != "https://example.com/two" || recent[0].Title != "Two" {
		t.Fatalf("newest = %+v, want two/Two", recent[0])
	}
	if recent[1].URL != "https://example.com/one" || recent[1].Title != "One" {
		t.Fatalf("oldest = %+v, want one/One", recent[1])
	}
}
