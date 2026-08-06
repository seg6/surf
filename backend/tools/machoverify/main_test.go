package main

import "testing"

func TestVersionRoundTrip(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "12", want: "12.0.0"},
		{input: "12.0", want: "12.0.0"},
		{input: "14.5.1", want: "14.5.1"},
	} {
		encoded, err := parseVersion(test.input)
		if err != nil {
			t.Fatalf("parseVersion(%q): %v", test.input, err)
		}
		if got := formatVersion(encoded); got != test.want {
			t.Fatalf("formatVersion(parseVersion(%q))=%q, want %q",
				test.input, got, test.want)
		}
	}
}

func TestParseVersionRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "12.", "12.0.0.1", "x", "12.256"} {
		if _, err := parseVersion(input); err == nil {
			t.Errorf("parseVersion(%q) unexpectedly succeeded", input)
		}
	}
}
