package editor

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sahilm/fuzzy"
)

const filePickerLimit = 50

type FilePicker struct {
	root   string
	files  []string // relative paths
	query  string
	items  []string
	sel    int
	primed bool
}

func NewFilePicker(root string) *FilePicker {
	fp := &FilePicker{root: root}
	fp.scan()
	return fp
}

func (f *FilePicker) scan() {
	var paths []string
	_ = filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(f.root, p)
		if rel == "." {
			return nil
		}
		// Skip hidden dirs and .git etc unless queried.
		for _, seg := range strings.Split(rel, string(filepath.Separator)) {
			if strings.HasPrefix(seg, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, rel)
		if len(paths) > 5000 {
			return filepath.SkipAll
		}
		return nil
	})
	f.files = paths
}

func (f *FilePicker) SetQuery(q string) {
	if f.primed && f.query == q {
		return
	}
	f.primed = true
	f.query = q
	f.sel = 0
	if q == "" {
		if len(f.files) > filePickerLimit {
			f.items = f.files[:filePickerLimit]
		} else {
			f.items = append([]string{}, f.files...)
		}
		return
	}
	if strings.HasPrefix(q, ".") {
		// include hidden
		all := []string{}
		_ = filepath.WalkDir(f.root, func(p string, d fs.DirEntry, _ error) error {
			if d == nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(f.root, p)
			all = append(all, rel)
			return nil
		})
		f.items = filterFuzzy(all, q, filePickerLimit)
		return
	}
	f.items = filterFuzzy(f.files, q, filePickerLimit)
}

func filterFuzzy(corpus []string, q string, limit int) []string {
	matches := fuzzy.Find(q, corpus)
	out := make([]string, 0, limit)
	for i, m := range matches {
		if i >= limit {
			break
		}
		out = append(out, m.Str)
	}
	return out
}

func (f *FilePicker) Items() []string { return f.items }
func (f *FilePicker) Sel() int        { return f.sel }
func (f *FilePicker) Selected() string {
	if f.sel < 0 || f.sel >= len(f.items) {
		return ""
	}
	return f.items[f.sel]
}
func (f *FilePicker) Up() {
	if f.sel > 0 {
		f.sel--
	}
}
func (f *FilePicker) Down() {
	if f.sel < len(f.items)-1 {
		f.sel++
	}
}
