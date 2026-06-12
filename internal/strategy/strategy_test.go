package strategy

import "testing"

func TestParseValidStrategy(t *testing.T) {
	got, err := Parse("jp-10")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Region != "jp" {
		t.Fatalf("Region = %q, want jp", got.Region)
	}
	if got.RotateMinutes != 10 {
		t.Fatalf("RotateMinutes = %d, want 10", got.RotateMinutes)
	}
	if got.Key() != "jp-10" {
		t.Fatalf("Key = %q, want jp-10", got.Key())
	}
}

func TestParseFixedStrategy(t *testing.T) {
	got, err := Parse("US-0")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Region != "us" {
		t.Fatalf("Region = %q, want us", got.Region)
	}
	if got.RotateMinutes != 0 {
		t.Fatalf("RotateMinutes = %d, want 0", got.RotateMinutes)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	cases := []string{"", "jp", "jp-", "-10", "japan-10", "jp-ten", "jp--10", "jp-100000"}
	for _, tc := range cases {
		if _, err := Parse(tc); err == nil {
			t.Fatalf("Parse(%q) returned nil error", tc)
		}
	}
}
