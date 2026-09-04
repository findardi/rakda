package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeComponent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Akta Pendirian", "Akta Pendirian"},
		{"Laporan/2024", "Laporan-2024"},
		{`a\b:c*d?e"f<g>h|i`, "a-b-c-d-e-f-g-h-i"},
		{"  spasi   ganda  ", "spasi ganda"},
		{".", "-"},
		{"..", "-"},
		{"...", "-"},
		{"", "-"},
		{"   ", "-"},
		{"trailing dots...", "trailing dots"},
		{"Anggaran Dasar\x00\x1f", "Anggaran Dasar"},
		{"Ringkasan Rapat 📎", "Ringkasan Rapat 📎"},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, sanitizeComponent(c.in), "input %q", c.in)
	}
}

func TestSanitizeComponentNeverExceedsBudget(t *testing.T) {
	long := strings.Repeat("é", 400)
	got := sanitizeComponent(long)

	assert.LessOrEqual(t, len(got), maxComponentBytes)
	assert.True(t, strings.HasPrefix(long, got), "harus dipotong di batas rune, bukan di tengah")
}

func TestSanitizeFileNameKeepsExtension(t *testing.T) {
	long := strings.Repeat("a", 300) + ".pdf"
	got := sanitizeFileName(long)

	assert.True(t, strings.HasSuffix(got, ".pdf"), "got %q", got)
	assert.LessOrEqual(t, len(got), maxComponentBytes)

	assert.Equal(t, "laporan-q1.pdf", sanitizeFileName("laporan/q1.pdf"))
	assert.Equal(t, "-.pdf", sanitizeFileName("...pdf"))
}

func TestDedupName(t *testing.T) {
	used := map[string]struct{}{}

	assert.Equal(t, "akta.pdf", dedupName(used, "akta.pdf"))
	assert.Equal(t, "akta (2).pdf", dedupName(used, "akta.pdf"))
	assert.Equal(t, "akta (3).pdf", dedupName(used, "akta.pdf"))

	// Windows dan macOS tidak membedakan huruf besar/kecil: keduanya bertabrakan.
	assert.Equal(t, "AKTA (4).pdf", dedupName(used, "AKTA.pdf"))

	assert.Equal(t, "Korporasi", dedupName(used, "Korporasi"))
	assert.Equal(t, "Korporasi (2)", dedupName(used, "Korporasi"))
}

func TestArchiveNumber(t *testing.T) {
	assert.Equal(t, "01", archiveNumber("", 1, 9))
	assert.Equal(t, "09", archiveNumber("", 9, 9))
	assert.Equal(t, "01.02", archiveNumber("01", 2, 5))
	assert.Equal(t, "01.02.03", archiveNumber("01.02", 3, 5))

	// Lebar menyesuaikan supaya urutan leksikografis tetap sama dengan position.
	assert.Equal(t, "007", archiveNumber("", 7, 100))
	assert.Equal(t, "0007", archiveNumber("", 7, 1000))
}

func TestResolveArchivePathFitsWithoutShortening(t *testing.T) {
	got, relocated := resolveArchivePath(archivePath{
		root:      "ruang-arsip-2026-08-27/dokumen",
		dirs:      []string{"01 Korporasi", "01.02 Anggaran dasar"},
		dirsPlain: []string{"Korporasi", "Anggaran dasar"},
		file:      "01.02.03 Akta pendirian.pdf",
		filePlain: "Akta pendirian.pdf",
	})

	assert.False(t, relocated)
	assert.Equal(t,
		"ruang-arsip-2026-08-27/dokumen/01 Korporasi/01.02 Anggaran dasar/01.02.03 Akta pendirian.pdf",
		got)
}

func TestResolveArchivePathClimbsTheLadder(t *testing.T) {
	deep := []string{}
	deepPlain := []string{}
	for range 4 {
		deep = append(deep, "01 "+strings.Repeat("panjang", 8))
		deepPlain = append(deepPlain, strings.Repeat("panjang", 8))
	}

	got, relocated := resolveArchivePath(archivePath{
		root:      "ruang-arsip-2026-08-27/dokumen",
		dirs:      deep,
		dirsPlain: deepPlain,
		file:      "01.02.03.04.05 " + strings.Repeat("berkas", 10) + ".pdf",
		filePlain: strings.Repeat("berkas", 10) + ".pdf",
	})

	require.LessOrEqual(t, len([]rune(got)), maxZipPathChars)
	assert.False(t, relocated, "tangga harus menyelesaikannya tanpa membuang ke folder khusus")
	assert.True(t, strings.HasSuffix(got, ".pdf"), "ekstensi harus selamat: %q", got)
}

func TestResolveArchivePathRelocatesWhenHopeless(t *testing.T) {
	dirs := []string{}
	for range 12 {
		dirs = append(dirs, strings.Repeat("x", 60))
	}

	got, relocated := resolveArchivePath(archivePath{
		root:      strings.Repeat("r", 100) + "/dokumen",
		dirs:      dirs,
		dirsPlain: dirs,
		file:      "01 berkas.pdf",
		filePlain: "berkas.pdf",
	})

	assert.True(t, relocated)
	assert.Contains(t, got, relocatedDir)
	assert.True(t, strings.HasSuffix(got, ".pdf"))
}

func TestValidateNodeName(t *testing.T) {
	ok := []string{"Korporasi", "  Akta 2024  ", "Laporan (final).pdf", "Ringkasan 📎"}
	for _, in := range ok {
		got, valid := validateNodeName(in)
		assert.True(t, valid, "input %q", in)
		assert.Equal(t, strings.TrimSpace(in), got)
	}

	bad := []string{"", "   ", ".", "..", "a/b", `a\b`, "tab\there", strings.Repeat("x", 201)}
	for _, in := range bad {
		_, valid := validateNodeName(in)
		assert.False(t, valid, "input %q seharusnya ditolak", in)
	}
}

func TestArchiveRootName(t *testing.T) {
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		slug  string
		scope []string
		want  string
	}{
		{name: "whole room unchanged", slug: "ruang-1a2b3c4d", want: "ruang-1a2b3c4d-arsip-2026-09-04"},
		{name: "one folder, spaces and slash sanitized", slug: "ruang-1a2b3c4d", scope: []string{"Anggaran Dasar/2024"}, want: "ruang-1a2b3c4d-Anggaran-Dasar-2024-arsip-2026-09-04"},
		{name: "several folders", slug: "ruang-1a2b3c4d", scope: []string{"A", "B"}, want: "ruang-1a2b3c4d-arsip-sebagian-2026-09-04"},
		{name: "long slug leaves no room for a folder", slug: strings.Repeat("s", 110), scope: []string{"Keuangan"}, want: strings.Repeat("s", 110) + "-arsip-sebagian-2026-09-04"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, archiveRootName(tc.slug, at, tc.scope))
		})
	}

	t.Run("long folder name is truncated within the component budget", func(t *testing.T) {
		got := archiveRootName("ruang-1a2b3c4d", at, []string{strings.Repeat("k", 400)})
		assert.LessOrEqual(t, len(got), maxComponentBytes)
		assert.True(t, strings.HasSuffix(got, "-arsip-2026-09-04"), got)
		assert.True(t, strings.HasPrefix(got, "ruang-1a2b3c4d-kkk"), got)
	})
}
