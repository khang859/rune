package editor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// historyCap bounds the on-disk history file so it can't grow without limit.
const historyCap = 1000

// History stores submitted user prompts and supports shell-like up/down recall.
// Index semantics: entries[len-1] is the newest. nav==-1 means "not navigating";
// nav==i (0..len-1) means the textarea currently shows entries[len-1-i].
type History struct {
	path    string
	entries []string
	nav     int
	draft   string
}

func NewHistory(path string) *History {
	h := &History{path: path, nav: -1}
	h.load()
	return h
}

type historyLine struct {
	Text string `json:"text"`
}

func (h *History) load() {
	if h.path == "" {
		return
	}
	f, err := os.Open(h.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var hl historyLine
		if err := json.Unmarshal(sc.Bytes(), &hl); err != nil {
			continue
		}
		if hl.Text == "" {
			continue
		}
		h.entries = append(h.entries, hl.Text)
	}
}

// Push appends text to history (skipping empty / dedup adjacent), persists,
// and resets navigation state.
func (h *History) Push(text string) {
	h.nav = -1
	h.draft = ""
	if text == "" {
		return
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == text {
		return
	}
	h.entries = append(h.entries, text)
	if len(h.entries) > historyCap {
		h.entries = h.entries[len(h.entries)-historyCap:]
	}
	h.persist()
}

// Prev moves one step further back; returns the entry to display and true.
// On the first Prev call, current is captured as the draft so Next can restore it.
func (h *History) Prev(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.nav == -1 {
		h.draft = current
		h.nav = 0
		return h.entries[len(h.entries)-1], true
	}
	if h.nav+1 >= len(h.entries) {
		return h.entries[len(h.entries)-1-h.nav], true // already at oldest
	}
	h.nav++
	return h.entries[len(h.entries)-1-h.nav], true
}

// Next moves one step forward. Returns (text, true) for an entry, or (draft,
// true) when stepping past the newest entry. Returns ("", false) when not
// navigating.
func (h *History) Next() (string, bool) {
	if h.nav == -1 {
		return "", false
	}
	if h.nav == 0 {
		d := h.draft
		h.nav = -1
		h.draft = ""
		return d, true
	}
	h.nav--
	return h.entries[len(h.entries)-1-h.nav], true
}

// Reset clears navigation state without changing entries (call when the user
// edits the textarea so a later submit captures their actual input).
func (h *History) Reset() {
	h.nav = -1
	h.draft = ""
}

func (h *History) Navigating() bool { return h.nav != -1 }

func (h *History) persist() {
	if h.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), ".history-*")
	if err != nil {
		return
	}
	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	for _, e := range h.entries {
		_ = enc.Encode(historyLine{Text: e})
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), h.path)
}
