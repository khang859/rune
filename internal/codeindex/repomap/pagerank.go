package repomap

const (
	pagerankDamping       = 0.85
	pagerankMaxIterations = 50
	pagerankTolerance     = 1e-6
)

type WeightedEdge struct {
	From   string
	To     string
	Weight float64
}

// PageRank runs personalized PageRank over nodes and weighted directed edges.
// personalization may be nil (uniform). Returns nil for empty input.
func PageRank(nodes []string, edges []WeightedEdge, personalization map[string]float64) map[string]float64 {
	if len(nodes) == 0 {
		return nil
	}

	n := float64(len(nodes))
	outWeight := map[string]float64{}
	outEdges := map[string][]WeightedEdge{}
	for _, e := range edges {
		outEdges[e.From] = append(outEdges[e.From], e)
		outWeight[e.From] += e.Weight
	}

	// Normalize personalization to a probability distribution; default uniform.
	pers := map[string]float64{}
	if len(personalization) == 0 {
		for _, node := range nodes {
			pers[node] = 1.0 / n
		}
	} else {
		var sum float64
		for _, node := range nodes {
			sum += personalization[node]
		}
		if sum <= 0 {
			for _, node := range nodes {
				pers[node] = 1.0 / n
			}
		} else {
			for _, node := range nodes {
				pers[node] = personalization[node] / sum
			}
		}
	}

	score := map[string]float64{}
	for _, node := range nodes {
		score[node] = 1.0 / n
	}

	for iter := 0; iter < pagerankMaxIterations; iter++ {
		next := map[string]float64{}
		var dangling float64
		for _, node := range nodes {
			if outWeight[node] == 0 {
				dangling += score[node]
			}
		}
		for _, node := range nodes {
			// Teleport contribution: (1 - d) * pers + d * (dangling redistributed via pers).
			next[node] = (1.0-pagerankDamping)*pers[node] + pagerankDamping*dangling*pers[node]
		}
		for _, src := range nodes {
			if outWeight[src] == 0 {
				continue
			}
			for _, e := range outEdges[src] {
				next[e.To] += pagerankDamping * score[src] * (e.Weight / outWeight[src])
			}
		}

		var delta float64
		for _, node := range nodes {
			d := next[node] - score[node]
			if d < 0 {
				d = -d
			}
			delta += d
		}
		score = next
		if delta < pagerankTolerance {
			break
		}
	}
	return score
}
