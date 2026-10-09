package main

// ファイル探し: 「先週作った Excel どこ？」で、ホームのよく使うフォルダから名前・種類・日付で探して、新しい順に出す。
//   AI の <findfile ext="xlsx,xls" days="7">見積</findfile>、または /find <言葉> [.拡張子]
// node_modules・.git などの中身の多いフォルダは飛ばし、探すのは最大 4 秒・見つけるのは最大 10 件。

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	findBudget = 4 * time.Second
	findMax    = 10
)

type foundFile struct {
	Path string
	Mod  time.Time
	Size int64
}

// 探すフォルダ (ホームのよく使うところ。OneDrive も)
func findRoots() []string {
	home := homeDir()
	var roots []string
	for _, d := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Videos", "Music", "Movies"} {
		roots = append(roots, filepath.Join(home, d))
	}
	if od := os.Getenv("OneDrive"); od != "" {
		roots = append(roots, od)
	}
	entries, _ := os.ReadDir(home)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "OneDrive") {
			roots = append(roots, filepath.Join(home, e.Name()))
		}
	}
	// 同じ場所 (OneDrive の中のデスクトップなど) を二度探さない
	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		if st, err := os.Stat(r); err == nil && st.IsDir() && !seen[strings.ToLower(r)] {
			seen[strings.ToLower(r)] = true
			out = append(out, r)
		}
	}
	return out
}

var skipDirs = map[string]bool{"node_modules": true, ".git": true, "AppData": true, "$RECYCLE.BIN": true, ".cache": true, "__pycache__": true, ".venv": true, "venv": true, "Library": true}

// words を全部名前に含み、exts (なければ何でも) で、days 日以内 (0 は何日でも) に変わったファイル
func findFiles(words, exts []string, days int) []foundFile {
	deadline := time.Now().Add(findBudget)
	var since time.Time
	if days > 0 {
		since = time.Now().AddDate(0, 0, -days)
	}
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	extOK := map[string]bool{}
	for _, e := range exts {
		if e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), ".")); e != "" {
			extOK["."+e] = true
		}
	}
	var found []foundFile
	seen := map[string]bool{}
	for _, root := range findRoots() {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if time.Now().After(deadline) {
				return fs.SkipAll
			}
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if p != root && (skipDirs[name] || strings.HasPrefix(name, ".")) {
					return fs.SkipDir
				}
				return nil
			}
			lower := strings.ToLower(name)
			if len(extOK) > 0 && !extOK[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			for _, w := range words {
				if !strings.Contains(lower, w) {
					return nil
				}
			}
			info, err := d.Info()
			if err != nil || (!since.IsZero() && info.ModTime().Before(since)) || seen[strings.ToLower(p)] {
				return nil
			}
			seen[strings.ToLower(p)] = true
			found = append(found, foundFile{p, info.ModTime(), info.Size()})
			return nil
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Mod.After(found[j].Mod) })
	if len(found) > findMax {
		found = found[:findMax]
	}
	return found
}

func describeFound(list []foundFile) string {
	var b strings.Builder
	for i, f := range list {
		fmt.Fprintf(&b, "%d. %s  (%s, %s)\n", i+1, f.Path, f.Mod.Format("2006-01-02 15:04"), humanSize(f.Size))
	}
	return b.String()
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// AI の <findfile ext="…" days="…">言葉</findfile>
func (l *Lumi) findFileTool(r toolRequest) string {
	words := strings.Fields(r.Command)
	var exts []string
	if e := r.Attrs["ext"]; e != "" {
		exts = strings.Split(e, ",")
	}
	days, _ := strconv.Atoi(strings.TrimSpace(r.Attrs["days"]))
	l.write(T("find.searching")+"\n", "cyan")
	list := findFiles(words, exts, days)
	head := "\n[" + T("find.label") + "] " + strings.TrimSpace(r.Command+" "+r.Attrs["ext"]) + "\n"
	if len(list) == 0 {
		l.write("  "+T("find.none")+"\n\n", "dim")
		return head + T("find.none") + "\n"
	}
	text := describeFound(list)
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		l.write("  "+line+"\n", "dim")
	}
	l.write("\n", "fg")
	return head + text
}

// /find <言葉> [.拡張子] [7d]
func (l *Lumi) findCommand(args []string) {
	var words, exts []string
	days := 0
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "."):
			exts = append(exts, a)
		case strings.HasSuffix(a, "d") && len(a) > 1:
			if n, err := strconv.Atoi(strings.TrimSuffix(a, "d")); err == nil {
				days = n
				continue
			}
			words = append(words, a)
		default:
			words = append(words, a)
		}
	}
	if len(words) == 0 && len(exts) == 0 {
		l.info(T("find.how"))
		return
	}
	list := findFiles(words, exts, days)
	if len(list) == 0 {
		l.info(T("find.none"))
		return
	}
	l.info(strings.TrimRight(describeFound(list), "\n"))
}
