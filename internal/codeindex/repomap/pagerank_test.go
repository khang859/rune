package repomap

import (
	"math"
	"testing"
)

func TestPageRankUniformWithoutPersonalization(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	edges := []WeightedEdge{
		{From: "a", To: "b", Weight: 1},
		{From: "b", To: "c", Weight: 1},
		{From: "c", To: "d", Weight: 1},
		{From: "d", To: "a", Weight: 1},
	}
	scores := PageRank(nodes, edges, nil)
	if len(scores) != 4 {
		t.Fatalf("want 4 scores, got %d", len(scores))
	}
	// Symmetric ring graph: all scores should be ~0.25.
	for _, n := range nodes {
		if math.Abs(scores[n]-0.25) > 0.01 {
			t.Errorf("score[%s] = %f, want ~0.25", n, scores[n])
		}
	}
}

func TestPageRankPersonalizationShiftsRanks(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	edges := []WeightedEdge{
		{From: "a", To: "b", Weight: 1},
		{From: "b", To: "c", Weight: 1},
		{From: "c", To: "d", Weight: 1},
		{From: "d", To: "a", Weight: 1},
	}
	pers := map[string]float64{"a": 1.0}
	scores := PageRank(nodes, edges, pers)
	if scores["a"] <= scores["c"] {
		t.Errorf("personalized node a (%f) should rank above c (%f)", scores["a"], scores["c"])
	}
}

func TestPageRankEmptyGraphReturnsNil(t *testing.T) {
	scores := PageRank(nil, nil, nil)
	if scores != nil {
		t.Errorf("want nil for empty input, got %v", scores)
	}
}
