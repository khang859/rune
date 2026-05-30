package skill

import (
	"reflect"
	"testing"
)

func TestFilter(t *testing.T) {
	all := []Skill{
		{Slug: "web-research", Body: "wr"},
		{Slug: "summarize", Body: "sm"},
		{Slug: "unused", Body: "un"},
	}
	matched, missing := Filter(all, []string{"summarize", "web-research", "missing"})

	wantMatched := []Skill{{Slug: "summarize", Body: "sm"}, {Slug: "web-research", Body: "wr"}}
	if !reflect.DeepEqual(matched, wantMatched) {
		t.Errorf("matched = %v, want %v", matched, wantMatched)
	}
	if want := []string{"missing"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

func TestFilterEmptyNames(t *testing.T) {
	all := []Skill{{Slug: "a"}}
	matched, missing := Filter(all, nil)
	if matched != nil || missing != nil {
		t.Errorf("Filter(nil) = (%v, %v), want (nil, nil)", matched, missing)
	}
}
