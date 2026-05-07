package renameapi

import "testing"

func TestParse(t *testing.T) {
	got, err := Parse("42")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("Parse(\"42\") = %d, want 42", got)
	}
}

func TestDouble(t *testing.T) {
	got, err := Double("7")
	if err != nil {
		t.Fatal(err)
	}
	if got != 14 {
		t.Errorf("Double(\"7\") = %d, want 14", got)
	}
}

func TestTriple(t *testing.T) {
	got, err := Triple("5")
	if err != nil {
		t.Fatal(err)
	}
	if got != 15 {
		t.Errorf("Triple(\"5\") = %d, want 15", got)
	}
}
