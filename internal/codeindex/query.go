package codeindex

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type captureSet map[string][]treesitter.Node

func querySymbols(adapter LanguageAdapter, root *treesitter.Node, source []byte, file string) ([]*Symbol, error) {
	sets, err := runQuery(adapter.Language, adapter.DefinitionQuery, root, source)
	if err != nil {
		return nil, err
	}
	var out []*Symbol
	seen := map[string]bool{}
	for _, set := range sets {
		nameNode, ok := firstCapture(set, "name")
		if !ok {
			continue
		}
		kind, node, ok := definitionKind(set)
		if !ok {
			continue
		}
		name := strings.TrimSpace(nameNode.Utf8Text(source))
		if name == "" {
			continue
		}
		start := node.StartPosition()
		end := node.EndPosition()
		sig := signature(source, node)
		sym := &Symbol{
			Name:      name,
			Qualified: name,
			Kind:      kind,
			Language:  adapter.ID,
			File:      file,
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
			StartLine: start.Row + 1,
			EndLine:   end.Row + 1,
			Signature: sig,
		}
		sym.ID = symbolID(sym.Language, sym.File, sym.Qualified, sym.Kind, sym.StartLine)
		key := sym.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sym)
	}
	return out, nil
}

func queryImports(adapter LanguageAdapter, root *treesitter.Node, source []byte) ([]Import, error) {
	if strings.TrimSpace(adapter.ImportQuery) == "" {
		return nil, nil
	}
	sets, err := runQuery(adapter.Language, adapter.ImportQuery, root, source)
	if err != nil {
		return nil, err
	}
	var out []Import
	seen := map[string]bool{}
	for _, set := range sets {
		sourceNode, ok := firstCapture(set, "source")
		if !ok {
			continue
		}
		imp := Import{Source: unquote(sourceNode.Utf8Text(source))}
		if aliasNode, ok := firstCapture(set, "alias"); ok {
			imp.Alias = strings.TrimSpace(aliasNode.Utf8Text(source))
		}
		if imp.Source == "" || seen[imp.Source+"\x00"+imp.Alias] {
			continue
		}
		seen[imp.Source+"\x00"+imp.Alias] = true
		out = append(out, imp)
	}
	return out, nil
}

func queryReferences(adapter LanguageAdapter, root *treesitter.Node, source []byte, file string) ([]Reference, error) {
	if strings.TrimSpace(adapter.ReferenceQuery) == "" {
		return nil, nil
	}
	sets, err := runQuery(adapter.Language, adapter.ReferenceQuery, root, source)
	if err != nil {
		return nil, err
	}
	var out []Reference
	seen := map[string]bool{}
	for _, set := range sets {
		if node, ok := firstCapture(set, "call"); ok {
			name := referenceName(source, node)
			if name != "" {
				key := "call" + name + string(rune(node.StartByte()))
				if !seen[key] {
					seen[key] = true
					out = append(out, Reference{Name: name, Kind: RelCalls, File: file, StartByte: node.StartByte(), EndByte: node.EndByte()})
				}
			}
		}
		if node, ok := firstCapture(set, "reference"); ok {
			name := referenceName(source, node)
			if name != "" {
				key := "ref" + name + string(rune(node.StartByte()))
				if !seen[key] {
					seen[key] = true
					out = append(out, Reference{Name: name, Kind: RelReferences, File: file, StartByte: node.StartByte(), EndByte: node.EndByte()})
				}
			}
		}
	}
	return out, nil
}

func runQuery(lang *treesitter.Language, querySource string, root *treesitter.Node, source []byte) ([]captureSet, error) {
	query, qerr := treesitter.NewQuery(lang, querySource)
	if qerr != nil {
		return nil, qerr
	}
	defer query.Close()
	cursor := treesitter.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(query, root, source)
	names := query.CaptureNames()
	var out []captureSet
	for match := matches.Next(); match != nil; match = matches.Next() {
		set := captureSet{}
		for _, capture := range match.Captures {
			idx := int(capture.Index)
			if idx < 0 || idx >= len(names) {
				continue
			}
			name := names[idx]
			set[name] = append(set[name], capture.Node)
		}
		out = append(out, set)
	}
	return out, nil
}

func firstCapture(set captureSet, name string) (treesitter.Node, bool) {
	nodes := set[name]
	if len(nodes) == 0 {
		return treesitter.Node{}, false
	}
	return nodes[0], true
}

func definitionKind(set captureSet) (SymbolKind, treesitter.Node, bool) {
	for _, candidate := range []struct {
		capture string
		kind    SymbolKind
	}{
		{"function", SymbolFunction},
		{"method", SymbolMethod},
		{"class", SymbolClass},
		{"struct", SymbolStruct},
		{"interface", SymbolInterface},
		{"type", SymbolType},
		{"constant", SymbolConstant},
		{"variable", SymbolVariable},
	} {
		if node, ok := firstCapture(set, candidate.capture); ok {
			return candidate.kind, node, true
		}
	}
	return "", treesitter.Node{}, false
}

func signature(source []byte, node treesitter.Node) string {
	text := node.Utf8Text(source)
	if i := strings.Index(text, "\n"); i >= 0 {
		text = text[:i]
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 200 {
		text = text[:197] + "..."
	}
	return text
}

func referenceName(source []byte, node treesitter.Node) string {
	text := strings.TrimSpace(node.Utf8Text(source))
	if text == "" {
		return ""
	}
	text = strings.TrimSuffix(text, "()")
	if i := strings.LastIndex(text, "."); i >= 0 && i < len(text)-1 {
		return text[i+1:]
	}
	return text
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if s[0] == '\'' && s[len(s)-1] == '\'' || s[0] == '"' && s[len(s)-1] == '"' || s[0] == '`' && s[len(s)-1] == '`' {
			return s[1 : len(s)-1]
		}
	}
	return s
}
