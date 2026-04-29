package tui

import "os"

type IconMode string

const (
	IconModeAuto    IconMode = "auto"
	IconModeNerd    IconMode = "nerd"
	IconModeUnicode IconMode = "unicode"
	IconModeASCII   IconMode = "ascii"
)

type IconSet struct {
	App       string
	User      string
	Assistant string
	Thinking  string
	Tool      string
	OK        string
	Warning   string
	Error     string
	Cwd       string
	GitBranch string
	Session   string
	Tokens    string
	Context   string
	Summary   string
	Invoke    string
}

func DefaultIconMode() IconMode {
	switch IconMode(os.Getenv("RUNE_ICONS")) {
	case IconModeNerd:
		return IconModeNerd
	case IconModeASCII:
		return IconModeASCII
	case IconModeAuto:
		return IconModeAuto
	case IconModeUnicode:
		return IconModeUnicode
	default:
		return IconModeUnicode
	}
}

func IconSetForMode(mode string) IconSet {
	switch IconMode(mode) {
	case IconModeNerd:
		return IconSet{
			App:       "ᚱ",
			User:      "",
			Assistant: "󰬯",
			Thinking:  "",
			Tool:      "",
			OK:        "",
			Warning:   "",
			Error:     "",
			Cwd:       "",
			GitBranch: "",
			Session:   "",
			Tokens:    "󰆙",
			Context:   "󰊚",
			Summary:   "",
			Invoke:    "",
		}
	case IconModeASCII:
		return IconSet{
			App:       "R",
			User:      ">",
			Assistant: "@",
			Thinking:  "*",
			Tool:      "$",
			OK:        "+",
			Warning:   "!",
			Error:     "x",
			Cwd:       "dir",
			GitBranch: "git",
			Session:   "sess",
			Tokens:    "tok",
			Context:   "ctx",
			Summary:   "book",
			Invoke:    "*",
		}
	case IconModeAuto, IconModeUnicode, "":
		fallthrough
	default:
		return IconSet{
			App:       "ᚱ",
			User:      "✦",
			Assistant: "◈",
			Thinking:  "✦",
			Tool:      "⚒",
			OK:        "✓",
			Warning:   "!",
			Error:     "✕",
			Cwd:       "⌂",
			GitBranch: "⑂",
			Session:   "⑂",
			Tokens:    "#",
			Context:   "◷",
			Summary:   "◇",
			Invoke:    "✦",
		}
	}
}

func iconLabel(icon, label string) string {
	if icon == "" {
		return label
	}
	return icon + " " + label
}
