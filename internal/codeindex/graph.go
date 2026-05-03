package codeindex

type RelationKind string

const (
	RelDefines       RelationKind = "defines"
	RelContains      RelationKind = "contains"
	RelImports       RelationKind = "imports"
	RelCalls         RelationKind = "calls"
	RelReferences    RelationKind = "references"
	RelCallsName     RelationKind = "calls_name"
	RelReferenceName RelationKind = "references_name"
)

type Edge struct {
	From     string
	To       string
	Relation RelationKind
	Label    string
}

type Graph struct {
	Edges []Edge
	out   map[string][]Edge
	in    map[string][]Edge
}

func NewGraph() *Graph {
	return &Graph{out: map[string][]Edge{}, in: map[string][]Edge{}}
}

func (g *Graph) Add(from, to string, rel RelationKind, label string) {
	if from == "" || to == "" {
		return
	}
	e := Edge{From: from, To: to, Relation: rel, Label: label}
	g.Edges = append(g.Edges, e)
	g.out[from] = append(g.out[from], e)
	g.in[to] = append(g.in[to], e)
}

func (g *Graph) Out(id string) []Edge { return append([]Edge(nil), g.out[id]...) }
func (g *Graph) In(id string) []Edge  { return append([]Edge(nil), g.in[id]...) }

func FileID(path string) string     { return "file:" + path }
func ImportID(source string) string { return "import:" + source }
func NameID(name string) string     { return "name:" + name }
