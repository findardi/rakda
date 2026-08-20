package convert

import "testing"

func TestViewable(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{
		{"pdf", "laporan.pdf", true},
		{"pdf uppercase", "LAPORAN.PDF", true},
		{"docx", "kontrak.docx", true},
		{"xlsx", "model.xlsx", true},
		{"xls", "model.xls", true},
		{"xlsm", "model.xlsm", true},
		{"ods", "model.ods", true},
		{"csv", "export.csv", true},
		{"pptx", "deck.pptx", true},
		{"png", "logo.png", true},
		{"zip blocked", "arsip.zip", false},
		{"video blocked", "demo.mp4", false},
		{"audio blocked", "rapat.mp3", false},
		{"executable blocked", "setup.exe", false},
		{"no extension blocked", "README", false},
		{"double extension uses last", "laporan.pdf.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Viewable(tt.file); got != tt.want {
				t.Errorf("Viewable(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
