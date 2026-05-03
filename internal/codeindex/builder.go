package codeindex

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

const maxIndexFileBytes = 2 * 1024 * 1024

type BuildOptions struct {
	Root      string
	Languages []string
	MaxFiles  int
}

type Builder struct {
	adapters []LanguageAdapter
}

func NewBuilder() *Builder { return &Builder{adapters: BuiltinLanguages()} }

func (b *Builder) Build(ctx context.Context, opts BuildOptions) (*Index, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	idx := New(abs)
	allowed := map[string]bool{}
	for _, lang := range opts.Languages {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang != "" {
			allowed[lang] = true
		}
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 5000
	}

	count := 0
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != abs && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if count >= maxFiles {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		adapter, ok := AdapterForPath(rel, b.adapters)
		if !ok || (len(allowed) > 0 && !allowed[adapter.ID]) {
			return nil
		}
		if err := b.indexFile(ctx, idx, adapter, path, rel); err != nil {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return nil, err
	}
	resolveLocalCalls(idx)
	return idx, nil
}

func (b *Builder) indexFile(ctx context.Context, idx *Index, adapter LanguageAdapter, abs, rel string) error {
	info, err := os.Stat(abs)
	if err != nil || info.Size() > maxIndexFileBytes {
		return err
	}
	source, err := os.ReadFile(abs)
	if err != nil || isLikelyBinary(source) {
		return err
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(adapter.Language)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()
	root := tree.RootNode()
	file := &File{ID: FileID(rel), Path: rel, Language: adapter.ID}
	idx.Files[rel] = file

	definitions, err := querySymbols(adapter, root, source, rel)
	if err != nil {
		return err
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].StartByte != definitions[j].StartByte {
			return definitions[i].StartByte < definitions[j].StartByte
		}
		return definitions[i].EndByte > definitions[j].EndByte
	})
	assignParents(definitions)
	for _, sym := range definitions {
		idx.Symbols[sym.ID] = sym
		file.Symbols = append(file.Symbols, sym.ID)
		idx.Graph.Add(file.ID, sym.ID, RelDefines, "")
		if sym.ParentID != "" {
			idx.Graph.Add(sym.ParentID, sym.ID, RelContains, "")
		}
	}

	imports, err := queryImports(adapter, root, source)
	if err != nil {
		return err
	}
	file.Imports = imports
	for _, imp := range imports {
		idx.Graph.Add(file.ID, ImportID(imp.Source), RelImports, imp.Alias)
	}

	refs, err := queryReferences(adapter, root, source, rel)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		owner := enclosingSymbol(definitions, ref.StartByte, ref.EndByte)
		if owner == nil || owner.Name == ref.Name {
			continue
		}
		relKind := RelReferenceName
		if ref.Kind == RelCalls {
			relKind = RelCallsName
		}
		idx.Graph.Add(owner.ID, NameID(ref.Name), relKind, ref.Name)
	}
	return ctx.Err()
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", ".next", ".cache", "coverage":
		return true
	default:
		return false
	}
}

func isLikelyBinary(b []byte) bool {
	limit := len(b)
	if limit > 8000 {
		limit = 8000
	}
	for i := 0; i < limit; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

func assignParents(symbols []*Symbol) {
	for _, child := range symbols {
		var parent *Symbol
		for _, candidate := range symbols {
			if candidate == child || candidate.StartByte > child.StartByte || candidate.EndByte < child.EndByte {
				continue
			}
			if candidate.StartByte == child.StartByte && candidate.EndByte == child.EndByte {
				continue
			}
			if parent == nil || candidate.StartByte >= parent.StartByte && candidate.EndByte <= parent.EndByte {
				parent = candidate
			}
		}
		if parent != nil {
			child.ParentID = parent.ID
			child.Qualified = parent.Qualified + "." + child.Name
			child.ID = symbolID(child.Language, child.File, child.Qualified, child.Kind, child.StartLine)
		}
	}
}

func enclosingSymbol(symbols []*Symbol, start, end uint) *Symbol {
	var best *Symbol
	for _, sym := range symbols {
		if sym.StartByte <= start && sym.EndByte >= end {
			if best == nil || sym.StartByte >= best.StartByte && sym.EndByte <= best.EndByte {
				best = sym
			}
		}
	}
	return best
}

func resolveLocalCalls(idx *Index) {
	byName := map[string][]*Symbol{}
	for _, sym := range idx.Symbols {
		addNameIndex(byName, sym.Name, sym)
		addNameIndex(byName, sym.Qualified, sym)
	}
	edges := append([]Edge(nil), idx.Graph.Edges...)
	for _, edge := range edges {
		if edge.Relation != RelCallsName && edge.Relation != RelReferenceName {
			continue
		}
		matches := append([]*Symbol(nil), byName[edge.Label]...)
		if len(matches) == 0 {
			continue
		}
		from := idx.Symbols[edge.From]
		if from == nil {
			continue
		}
		var sameFile []*Symbol
		for _, match := range matches {
			if match.File == from.File {
				sameFile = append(sameFile, match)
			}
		}
		if len(sameFile) == 1 {
			matches = sameFile
		}
		if len(matches) != 1 {
			var sameLang []*Symbol
			for _, match := range matches {
				if match.Language == from.Language {
					sameLang = append(sameLang, match)
				}
			}
			if len(sameLang) == 1 {
				matches = sameLang
			}
		}
		if len(matches) != 1 {
			continue
		}
		rel := RelReferences
		if edge.Relation == RelCallsName {
			rel = RelCalls
		}
		idx.Graph.Add(edge.From, matches[0].ID, rel, edge.Label)
	}
}

func addNameIndex(byName map[string][]*Symbol, name string, sym *Symbol) {
	if name == "" {
		return
	}
	for _, existing := range byName[name] {
		if existing.ID == sym.ID {
			return
		}
	}
	byName[name] = append(byName[name], sym)
}

func symbolID(lang, file, qualified string, kind SymbolKind, line uint) string {
	name := strings.NewReplacer(" ", "_", "\t", "_", "\n", "_", ":", "_").Replace(qualified)
	return fmt.Sprintf("symbol:%s:%s:%s:%s:%d", lang, file, kind, name, line)
}
