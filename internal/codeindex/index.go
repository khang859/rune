package codeindex

import "sort"

type Index struct {
	Root    string
	Files   map[string]*File
	Symbols map[string]*Symbol
	Graph   *Graph
}

type File struct {
	ID       string
	Path     string
	Language string
	Package  string
	Symbols  []string
	Imports  []Import
}

type Import struct {
	Source string
	Alias  string
}

type Symbol struct {
	ID        string
	Name      string
	Qualified string
	Kind      SymbolKind
	Language  string
	File      string
	StartByte uint
	EndByte   uint
	StartLine uint
	EndLine   uint
	Signature string
	ParentID  string
}

type SymbolKind string

const (
	SymbolFunction  SymbolKind = "function"
	SymbolMethod    SymbolKind = "method"
	SymbolClass     SymbolKind = "class"
	SymbolStruct    SymbolKind = "struct"
	SymbolInterface SymbolKind = "interface"
	SymbolType      SymbolKind = "type"
	SymbolVariable  SymbolKind = "variable"
	SymbolConstant  SymbolKind = "constant"
)

type Reference struct {
	Name      string
	Kind      RelationKind
	File      string
	StartByte uint
	EndByte   uint
}

func New(root string) *Index {
	return &Index{
		Root:    root,
		Files:   map[string]*File{},
		Symbols: map[string]*Symbol{},
		Graph:   NewGraph(),
	}
}

func (idx *Index) SortedFiles() []*File {
	files := make([]*File, 0, len(idx.Files))
	for _, f := range idx.Files {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func (idx *Index) SortedSymbols() []*Symbol {
	syms := make([]*Symbol, 0, len(idx.Symbols))
	for _, s := range idx.Symbols {
		syms = append(syms, s)
	}
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].File != syms[j].File {
			return syms[i].File < syms[j].File
		}
		if syms[i].StartLine != syms[j].StartLine {
			return syms[i].StartLine < syms[j].StartLine
		}
		return syms[i].Qualified < syms[j].Qualified
	})
	return syms
}
