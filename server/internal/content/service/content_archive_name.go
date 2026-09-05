package service

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxComponentBytes = 120
	maxZipPathChars   = 180
	minShortNameBytes = 24
	relocatedDir      = "_PATH-TERLALU-PANJANG"
)

var (
	invalidNameChars = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]`)
	repeatedSpace    = regexp.MustCompile(`\s+`)
)

func sanitizeComponent(name string) string {
	s := invalidNameChars.ReplaceAllString(name, "-")
	s = repeatedSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "-. ")

	if s == "" {
		return "-"
	}

	return truncateBytes(s, maxComponentBytes)
}

// archiveRootName adalah nama direktori akar di dalam ZIP sekaligus nama
// berkas unduhan. Seluruh ruang: "{slug}-arsip-{tanggal}" (tidak berubah).
// Satu folder: "{slug}-{folder}-arsip-{tanggal}" — spasi pada nama folder
// menjadi "-", dan nama dipotong agar keseluruhan tetap di bawah
// maxComponentBytes; bila slug tidak menyisakan minShortNameBytes untuk
// folder, jatuh ke bentuk jamak. Beberapa folder:
// "{slug}-arsip-sebagian-{tanggal}"; daftar foldernya ada di BACA-DULU.
func archiveRootName(slug string, at time.Time, scopeNames []string) string {
	base := sanitizeComponent(slug)
	date := at.Format("2006-01-02")

	switch len(scopeNames) {
	case 0:
		return base + "-arsip-" + date
	case 1:
		budget := maxComponentBytes - len(base) - len("-") - len("-arsip-") - len(date)
		if budget >= minShortNameBytes {
			folder := strings.ReplaceAll(sanitizeComponent(scopeNames[0]), " ", "-")
			return base + "-" + truncateBytes(folder, budget) + "-arsip-" + date
		}
		return base + "-arsip-sebagian-" + date
	default:
		return base + "-arsip-sebagian-" + date
	}
}

func sanitizeFileName(name string) string {
	ext := path.Ext(name)
	base := name
	if ext != "" {
		base = name[:len(name)-len(ext)]
	}

	ext = strings.ToLower(invalidNameChars.ReplaceAllString(ext, "-"))
	ext = strings.TrimRight(ext, ". ")
	if len(ext) > 16 {
		ext = ext[:16]
	}

	return truncateBytes(sanitizeComponent(base), max(maxComponentBytes-len(ext), 1)) + ext
}

func truncateBytes(s string, limit int) string {
	limit = max(limit, 1)
	if len(s) <= limit {
		return s
	}

	b := []byte(s)
	for limit > 0 && !utf8.RuneStart(b[limit]) {
		limit--
	}

	out := strings.TrimRight(string(b[:limit]), "-. ")
	if out == "" {
		return "-"
	}

	return out
}

func shortenFileName(name string, budget int) string {
	ext := path.Ext(name)
	base := name
	if ext != "" {
		base = name[:len(name)-len(ext)]
	}

	return truncateBytes(base, max(budget-len(ext), 1)) + ext
}

// dedupName menjamin nama unik dalam satu direktori. Perbandingan
// case-insensitive: dua nama yang hanya beda huruf besar/kecil bertabrakan saat
// diekstrak di Windows dan macOS.
func dedupName(used map[string]struct{}, name string) string {
	if _, taken := used[strings.ToLower(name)]; !taken {
		used[strings.ToLower(name)] = struct{}{}
		return name
	}

	ext := path.Ext(name)
	base := name
	if ext != "" {
		base = name[:len(name)-len(ext)]
	}

	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, taken := used[strings.ToLower(candidate)]; !taken {
			used[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
}

type archivePath struct {
	root      string
	dirs      []string
	dirsPlain []string
	file      string
	filePlain string
}

func joinPath(root string, dirs []string, file string) string {
	parts := make([]string, 0, len(dirs)+2)
	if root != "" {
		parts = append(parts, root)
	}
	parts = append(parts, dirs...)
	parts = append(parts, file)
	return strings.Join(parts, "/")
}

func shortenDirs(dirs []string, budget int) []string {
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = truncateBytes(d, budget)
	}
	return out
}

// resolveArchivePath menerapkan tangga pemendekan yang disepakati 13-b. Nilai
// balik kedua menandai entri yang tidak muat dan dibuang ke folder khusus; path
// aslinya tetap tercatat di indeks.
func resolveArchivePath(p archivePath) (string, bool) {
	candidates := []string{
		joinPath(p.root, p.dirs, p.file),
		joinPath(p.root, p.dirs, p.filePlain),
		joinPath(p.root, p.dirs, shortenFileName(p.filePlain, minShortNameBytes*2)),
		joinPath(p.root, shortenDirs(p.dirs, minShortNameBytes), shortenFileName(p.filePlain, minShortNameBytes*2)),
		joinPath(p.root, shortenDirs(p.dirsPlain, minShortNameBytes), shortenFileName(p.filePlain, minShortNameBytes*2)),
	}

	for _, c := range candidates {
		if utf8.RuneCountInString(c) <= maxZipPathChars {
			return c, false
		}
	}

	return joinPath(p.root, []string{relocatedDir}, shortenFileName(p.filePlain, minShortNameBytes*2)), true
}

// archiveNumber merender nomor kumulatif bertitik. Lebarnya menyesuaikan jumlah
// saudara supaya urutan leksikografis di ekstraktor tetap sama dengan urutan
// position.
func archiveNumber(prefix string, index, siblings int) string {
	width := 2
	for pow := 100; siblings >= pow; pow *= 10 {
		width++
	}

	segment := fmt.Sprintf("%0*d", width, index)
	if prefix == "" {
		return segment
	}

	return prefix + "." + segment
}
