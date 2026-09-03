package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxPageTextBytes    = 256 << 10
	ocrMinContentLength = 20
)

func (s *ContentService) RunTextSweeper(ctx context.Context, interval time.Duration, batch int) {
	s.sweepTextOnce(ctx, batch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepTextOnce(ctx, batch)
		}
	}
}

func (s *ContentService) sweepTextOnce(ctx context.Context, batch int) {
	pending, err := s.repo.ListPendingTextExtraction(ctx, int32(batch))
	if err != nil {
		log.Printf("text sweep: list pending: %v", err)
		return
	}

	extracted, failed := 0, 0
	for _, v := range pending {
		if err := s.extractVersionText(ctx, v); err != nil {
			log.Printf("text sweep: extract version %s: %v", uuidString(v.ID), err)
			failed++
			continue
		}
		extracted++
	}

	if extracted > 0 || failed > 0 {
		log.Printf("text sweep: extracted %d versions, failed %d", extracted, failed)
	}
}

func (s *ContentService) extractVersionText(ctx context.Context, row contentdb.ListPendingTextExtractionRow) error {
	if row.RenditionKey == nil || row.PageCount == nil {
		return fmt.Errorf("version %s: %w", uuidString(row.ID), ErrRenditionPending)
	}

	renditionKey, pageCount := *row.RenditionKey, int(*row.PageCount)

	pdf, err := s.renditionGet(ctx, renditionKey)
	if err != nil {
		return fmt.Errorf("get rendition: %w", err)
	}
	defer pdf.Close()

	text, err := s.viewer.TextExtractor.ExtractText(ctx, pdf)
	if err != nil {
		return s.markTextFailed(ctx, row.ID, err)
	}

	pages := splitPageTexts(text, pageCount)

	err = s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
		if err := q.DeleteVersionPageText(ctx, row.ID); err != nil {
			return err
		}

		for i, content := range pages {
			if err := q.InsertPageText(ctx, contentdb.InsertPageTextParams{
				VersionID: row.ID,
				PageNo:    int32(i + 1),
				Content:   content,
			}); err != nil {
				return err
			}
		}

		return q.SetVersionTextExtracted(ctx, row.ID)
	})
	if err != nil {
		return fmt.Errorf("write page texts: %w", err)
	}

	return nil
}

func (s *ContentService) markTextFailed(ctx context.Context, versionID pgtype.UUID, cause error) error {
	msg := cause.Error()
	if err := s.repo.SetVersionTextFailure(ctx, contentdb.SetVersionTextFailureParams{
		TextError: &msg,
		ID:        versionID,
	}); err != nil {
		log.Printf("record text failure: %v", err)
	}

	return ErrTextExtractionFailed
}

// splitPageTexts membelah output pdftotext pada form feed dan menyejajarkan
// dengan jumlah halaman pdfinfo: selalu menghasilkan pageCount halaman,
// halaman kosong dipertahankan (sasaran OCR 9-e), ekor yang tidak terisi
// diisi string kosong.
func splitPageTexts(text string, pageCount int) []string {
	chunks := strings.Split(text, "\f")
	pages := make([]string, 0, pageCount)

	for i := 0; i < pageCount; i++ {
		content := ""
		if i < len(chunks) {
			content = chunks[i]
		}

		pages = append(pages, truncatePageContent(strings.TrimSpace(content)))
	}

	return pages
}

// truncatePageContent memotong pada batas 256 KB tanpa membelah rune.
// to_tsvector menolak input > 1.048.575 byte dan kolom vektornya
// generated-stored, jadi penolakan itu jatuh saat INSERT — satu halaman
// patologis tidak boleh menggagalkan seluruh dokumen.
func truncatePageContent(content string) string {
	if len(content) <= maxPageTextBytes {
		return content
	}

	cut := 0
	for i := range content {
		if i > maxPageTextBytes {
			break
		}
		cut = i
	}

	return content[:cut]
}

// RunOCRSweeper mengenali halaman hasil pindai. Jatah dihitung per-HALAMAN,
// bukan per-versi (keputusan 9-e): 750 halaman × 3,2 s ≈ 40 menit untuk satu
// dokumen, jadi sweeper harus bisa berhenti di tengah dokumen dan
// melanjutkannya di sapuan berikutnya.
func (s *ContentService) RunOCRSweeper(ctx context.Context, interval time.Duration, batch int) {
	s.sweepOCROnce(ctx, batch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOCROnce(ctx, batch)
		}
	}
}

func (s *ContentService) sweepOCROnce(ctx context.Context, batch int) {
	pending, err := s.repo.ListPendingOCRPages(ctx, int32(batch))
	if err != nil {
		log.Printf("ocr sweep: list pending: %v", err)
		return
	}

	done, failed := 0, 0
	groups := groupPagesByVersion(pending, func(r contentdb.ListPendingOCRPagesRow) pgtype.UUID { return r.VersionID })

	for _, group := range groups {
		d, f := s.ocrVersion(ctx, group)
		done += d
		failed += f
	}

	if done > 0 || failed > 0 {
		log.Printf("ocr sweep: %d pages recognized, %d failed", done, failed)
	}
}

func (s *ContentService) ocrVersion(ctx context.Context, rows []contentdb.ListPendingOCRPagesRow) (int, int) {
	head := rows[0]
	if head.RenditionKey == nil {
		log.Printf("ocr sweep: version %s: no rendition", uuidString(head.VersionID))
		return 0, len(rows)
	}

	pdf, err := s.renditionGet(ctx, *head.RenditionKey)
	if err != nil {
		log.Printf("ocr sweep: get rendition %s: %v", uuidString(head.VersionID), err)
		return 0, len(rows)
	}
	defer pdf.Close()

	doc, err := s.viewer.OCR.OpenOCR(pdf)
	if err != nil {
		log.Printf("ocr sweep: open rendition %s: %v", uuidString(head.VersionID), err)
		return 0, len(rows)
	}
	defer doc.Close()

	done, failed := 0, 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}

		if err := s.ocrPage(ctx, doc, row); err != nil {
			log.Printf("ocr sweep: page %s#%d: %v", uuidString(row.VersionID), row.PageNo, err)
			failed++
			continue
		}

		done++
	}

	return done, failed
}

func (s *ContentService) ocrPage(ctx context.Context, doc render.OCRDocument, row contentdb.ListPendingOCRPagesRow) error {
	res, err := doc.OCRPage(ctx, int(row.PageNo))
	if err != nil {
		return s.markPageOCRFailed(ctx, row.VersionID, row.PageNo, err)
	}

	wordsJSON, err := json.Marshal(res.Words)
	if err != nil {
		return s.markPageOCRFailed(ctx, row.VersionID, row.PageNo, fmt.Errorf("marshal words: %w", err))
	}

	if err := s.repo.SetPageOCRResult(ctx, contentdb.SetPageOCRResultParams{
		VersionID: row.VersionID,
		PageNo:    row.PageNo,
		Content:   truncatePageContent(res.Text),
		Words:     wordsJSON,
	}); err != nil {
		return fmt.Errorf("write ocr result: %w", err)
	}

	return nil
}

func (s *ContentService) markPageOCRFailed(ctx context.Context, versionID pgtype.UUID, pageNo int32, cause error) error {
	msg := cause.Error()
	if err := s.repo.SetPageOCRFailure(ctx, contentdb.SetPageOCRFailureParams{
		VersionID: versionID,
		PageNo:    pageNo,
		OcrError:  &msg,
	}); err != nil {
		log.Printf("record ocr failure: %v", err)
	}

	return ErrOCRFailed
}

// RunBBoxSweeper mengisi koordinat kata untuk PDF berteks asli (pdftotext
// -bbox) — halaman `text_source = 'pdf'` yang words-nya masih kosong.
// Jatah per-halaman seperti OCR.
func (s *ContentService) RunBBoxSweeper(ctx context.Context, interval time.Duration, batch int) {
	s.sweepBBoxOnce(ctx, batch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepBBoxOnce(ctx, batch)
		}
	}
}

func (s *ContentService) sweepBBoxOnce(ctx context.Context, batch int) {
	pending, err := s.repo.ListPendingWordBoxes(ctx, int32(batch))
	if err != nil {
		log.Printf("bbox sweep: list pending: %v", err)
		return
	}

	done, failed := 0, 0
	groups := groupPagesByVersion(pending, func(r contentdb.ListPendingWordBoxesRow) pgtype.UUID { return r.VersionID })

	for _, group := range groups {
		d, f := s.bboxVersion(ctx, group)
		done += d
		failed += f
	}

	if done > 0 || failed > 0 {
		log.Printf("bbox sweep: %d pages boxed, %d failed", done, failed)
	}
}

func (s *ContentService) bboxVersion(ctx context.Context, rows []contentdb.ListPendingWordBoxesRow) (int, int) {
	head := rows[0]
	if head.RenditionKey == nil {
		log.Printf("bbox sweep: version %s: no rendition", uuidString(head.VersionID))
		return 0, len(rows)
	}

	pdf, err := s.renditionGet(ctx, *head.RenditionKey)
	if err != nil {
		log.Printf("bbox sweep: get rendition %s: %v", uuidString(head.VersionID), err)
		return 0, len(rows)
	}
	defer pdf.Close()

	doc, err := s.viewer.WordBoxes.OpenWordBoxes(pdf)
	if err != nil {
		log.Printf("bbox sweep: open rendition %s: %v", uuidString(head.VersionID), err)
		return 0, len(rows)
	}
	defer doc.Close()

	done, failed := 0, 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}

		if err := s.bboxPage(ctx, doc, row); err != nil {
			log.Printf("bbox sweep: page %s#%d: %v", uuidString(row.VersionID), row.PageNo, err)
			failed++
			continue
		}

		done++
	}

	return done, failed
}

func groupPagesByVersion[T any](rows []T, versionID func(T) pgtype.UUID) [][]T {
	var groups [][]T

	for _, row := range rows {
		if n := len(groups); n > 0 && versionID(groups[n-1][0]) == versionID(row) {
			groups[n-1] = append(groups[n-1], row)
			continue
		}

		groups = append(groups, []T{row})
	}

	return groups
}

func (s *ContentService) bboxPage(ctx context.Context, doc render.WordBoxDocument, row contentdb.ListPendingWordBoxesRow) error {
	words, err := doc.ExtractWordBoxes(ctx, int(row.PageNo))
	if err != nil {
		// Catat error lewat kolom ocr_error/ocr_at dan tutup kandidat dengan
		// words='[]' — "sekali gagal tidak diulang" (pola 9-b/9-e).
		if werr := s.repo.SetPageWordBoxes(ctx, contentdb.SetPageWordBoxesParams{
			VersionID: row.VersionID,
			PageNo:    row.PageNo,
			Words:     []byte("[]"),
		}); werr != nil {
			log.Printf("bbox sweep: seal page %s#%d: %v", uuidString(row.VersionID), row.PageNo, werr)
		}
		return s.markPageOCRFailed(ctx, row.VersionID, row.PageNo, err)
	}

	wordsJSON, err := json.Marshal(words)
	if err != nil {
		return s.markPageOCRFailed(ctx, row.VersionID, row.PageNo, fmt.Errorf("marshal words: %w", err))
	}

	if err := s.repo.SetPageWordBoxes(ctx, contentdb.SetPageWordBoxesParams{
		VersionID: row.VersionID,
		PageNo:    row.PageNo,
		Words:     wordsJSON,
	}); err != nil {
		return fmt.Errorf("write word boxes: %w", err)
	}

	return nil
}
