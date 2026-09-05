package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ArchiveStatusPending = "pending"
	ArchiveStatusReady   = "ready"

	archiveStalePendingAge = 3 * time.Hour
	archiveDocumentsDir    = "dokumen"
)

var (
	ErrArchiveNotFound      = errors.New("archive not found")
	ErrArchiveAlreadyQueued = errors.New("an archive is already being prepared for this room")
	ErrArchiveNotReady      = errors.New("archive is not ready")
	ErrArchiveBusy          = errors.New("archive export is busy, try again shortly")
)

func archiveObjectKey(workspaceID, archiveID string) string {
	return fmt.Sprintf("archives/%s/%s.zip", workspaceID, archiveID)
}

func archiveResponse(row contentdb.WorkspaceArchive) dto.ArchiveResponse {
	res := dto.ArchiveResponse{
		ID:              row.ID.String(),
		Status:          row.Status,
		RequestedBy:     row.RequestedBy.String(),
		RequestedByName: row.RequestedByName,
		SizeBytes:       row.SizeBytes,
		ChecksumSHA256:  row.ChecksumSha256,
		DocumentCount:   row.DocumentCount,
		MissingCount:    row.MissingCount,
		Error:           row.Error,
		CreatedAt:       row.CreatedAt.Time,
		ExpiresAt:       row.ExpiresAt.Time,
		Scope:           dto.ArchiveScopeRoom,
	}

	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		res.CompletedAt = &t
	}

	if len(row.ScopeFolderIds) > 0 {
		res.Scope = dto.ArchiveScopeFolders
		res.ScopeFolderIDs = make([]string, 0, len(row.ScopeFolderIds))
		for _, id := range row.ScopeFolderIds {
			res.ScopeFolderIDs = append(res.ScopeFolderIDs, id.String())
		}
		res.ScopeFolderNames = append([]string(nil), row.ScopeFolderNames...)
	}

	return res
}

// archiveScope adalah cakupan yang sudah divalidasi. ids nil = seluruh ruang;
// keduanya selalu sejajar (dijaga juga oleh check constraint di basis data).
type archiveScope struct {
	ids   []pgtype.UUID
	names []string
}

// resolveArchiveScope memvalidasi setiap id terhadap folder hidup ruangan
// sebelum ada yang ditulis — validate-all-first seperti BulkDeleteFolders: id
// yang tidak ada atau milik ruangan lain adalah ErrFolderNotFound. Id ganda
// dibuang dengan urutan permintaan dipertahankan; folder yang bersarang di
// bawah folder terpilih lain dibiarkan — walk menulis subtree-nya sekali saja.
// Sumbernya ListArchiveFolders, kueri yang sama yang dipakai perakitan, supaya
// tidak ada dua definisi "folder hidup" yang bisa berbeda.
func (s *ContentService) resolveArchiveScope(ctx context.Context, wID pgtype.UUID, folderIDs []string) (archiveScope, error) {
	if len(folderIDs) == 0 {
		return archiveScope{}, nil
	}

	ids, err := scanUniqueUUIDs(folderIDs)
	if err != nil {
		return archiveScope{}, ErrFolderNotFound
	}

	folders, err := s.repo.ListArchiveFolders(ctx, wID)
	if err != nil {
		return archiveScope{}, fmt.Errorf("list archive folders: %w", err)
	}

	nameByID := make(map[string]string, len(folders))
	for _, f := range folders {
		nameByID[f.ID.String()] = f.Name
	}

	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name, ok := nameByID[id.String()]
		if !ok {
			return archiveScope{}, ErrFolderNotFound
		}
		names = append(names, name)
	}

	return archiveScope{ids: ids, names: names}, nil
}

// archiveScopeSet mengubah kolom cakupan baris menjadi set kunci uuidString.
// nil berarti seluruh ruang — pembeda yang dipakai walk dan penulis audit.
func archiveScopeSet(ids []pgtype.UUID) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id.String()] = struct{}{}
	}

	return set
}

// archiveScopeMissing menyebut folder cakupan yang tidak lagi ditemui saat
// perakitan (dihapus di antara validasi dan build). Namanya dari potret baris,
// jadi tetap terbaca meski foldernya sudah tidak ada.
func archiveScopeMissing(row contentdb.WorkspaceArchive, seen map[string]struct{}) []string {
	var missing []string
	for i, id := range row.ScopeFolderIds {
		if _, ok := seen[id.String()]; ok {
			continue
		}

		name := id.String()
		if i < len(row.ScopeFolderNames) {
			name = row.ScopeFolderNames[i]
		}
		missing = append(missing, name)
	}

	return missing
}

func archiveScopeMetadata(row contentdb.WorkspaceArchive) map[string]any {
	if len(row.ScopeFolderIds) == 0 {
		return map[string]any{"scope": dto.ArchiveScopeRoom}
	}

	return map[string]any{
		"scope":        dto.ArchiveScopeFolders,
		"folder_count": len(row.ScopeFolderIds),
		"folder_names": append([]string(nil), row.ScopeFolderNames...),
	}
}

func (s *ContentService) CreateArchive(ctx context.Context, workspaceID string, actor Actor, req dto.CreateArchiveRequest) (dto.ArchiveResponse, error) {
	if !actor.managesRoom() {
		return dto.ArchiveResponse{}, ErrContentForbidden
	}

	var wID, uID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return dto.ArchiveResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.ArchiveResponse{}, ErrContentForbidden
	}

	slug, err := s.repo.GetWorkspaceSlugForArchive(ctx, wID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.ArchiveResponse{}, ErrArchiveNotFound
	}
	if err != nil {
		return dto.ArchiveResponse{}, fmt.Errorf("get workspace slug: %w", err)
	}

	// Cakupan divalidasi sebelum pemeriksaan pending dan semafor: permintaan
	// yang salah dijawab sebagai kesalahan klien apa pun keadaan antrean.
	scope, err := s.resolveArchiveScope(ctx, wID, req.FolderIDs)
	if err != nil {
		return dto.ArchiveResponse{}, err
	}

	pending, err := s.repo.CountPendingArchives(ctx, wID)
	if err != nil {
		return dto.ArchiveResponse{}, fmt.Errorf("count pending archives: %w", err)
	}
	if pending > 0 {
		return dto.ArchiveResponse{}, ErrArchiveAlreadyQueued
	}

	select {
	case s.archiveSem <- struct{}{}:
	default:
		return dto.ArchiveResponse{}, ErrArchiveBusy
	}

	row, err := s.repo.CreateArchive(ctx, contentdb.CreateArchiveParams{
		WorkspaceID:      wID,
		RequestedBy:      uID,
		RequestedByName:  actor.Name,
		ExpiresAt:        pgtype.Timestamptz{Time: time.Now().Add(s.archiveTTL), Valid: true},
		ScopeFolderIds:   scope.ids,
		ScopeFolderNames: scope.names,
	})
	if err != nil {
		<-s.archiveSem
		return dto.ArchiveResponse{}, fmt.Errorf("create archive: %w", err)
	}

	archiveID := row.ID.String()
	key := archiveObjectKey(workspaceID, archiveID)

	if err := s.repo.SetArchiveObjectKey(ctx, contentdb.SetArchiveObjectKeyParams{
		ID:        row.ID,
		ObjectKey: key,
	}); err != nil {
		<-s.archiveSem
		return dto.ArchiveResponse{}, fmt.Errorf("set archive object key: %w", err)
	}
	row.ObjectKey = key

	// Sengaja lepas dari context request: perakitan berlangsung menit sementara
	// request selesai dalam milidetik. Kematian saat deploy ditangani sweeper,
	// yang menandai `pending` basi sebagai `failed` (precedent RetryRendition).
	s.wakeRenditionWorker()

	go func() {
		defer func() { <-s.archiveSem }()
		s.buildArchive(context.Background(), row, workspaceID, slug, actor)
	}()

	return archiveResponse(row), nil
}

func (s *ContentService) ListArchives(ctx context.Context, workspaceID string, actor Actor) ([]dto.ArchiveResponse, error) {
	if !actor.managesRoom() {
		return nil, ErrContentForbidden
	}

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("parse workspace id: %w", err)
	}

	rows, err := s.repo.ListArchives(ctx, wID)
	if err != nil {
		return nil, fmt.Errorf("list archives: %w", err)
	}

	res := make([]dto.ArchiveResponse, 0, len(rows))
	for _, r := range rows {
		res = append(res, archiveResponse(r))
	}

	return res, nil
}

type ArchiveObject struct {
	Key      string
	Size     int64
	FileName string
}

func (s *ContentService) GetArchiveObject(ctx context.Context, workspaceID, archiveID string, actor Actor) (ArchiveObject, error) {
	if !actor.managesRoom() {
		return ArchiveObject{}, ErrContentForbidden
	}

	row, err := s.getArchive(ctx, workspaceID, archiveID)
	if err != nil {
		return ArchiveObject{}, err
	}

	if row.Status != ArchiveStatusReady {
		return ArchiveObject{}, ErrArchiveNotReady
	}

	slug, err := s.repo.GetWorkspaceSlugForArchive(ctx, row.WorkspaceID)
	if err != nil {
		return ArchiveObject{}, fmt.Errorf("get workspace slug: %w", err)
	}

	return ArchiveObject{
		Key:      row.ObjectKey,
		Size:     row.SizeBytes,
		FileName: archiveRootName(slug, row.CreatedAt.Time, row.ScopeFolderNames) + ".zip",
	}, nil
}

func (s *ContentService) OpenArchiveRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	return s.store.GetRange(ctx, key, offset, length)
}

func (s *ContentService) DeleteArchive(ctx context.Context, workspaceID, archiveID string, actor Actor) error {
	if !actor.managesRoom() {
		return ErrContentForbidden
	}

	row, err := s.getArchive(ctx, workspaceID, archiveID)
	if err != nil {
		return err
	}

	if row.ObjectKey != "" {
		if err := s.store.Delete(ctx, row.ObjectKey); err != nil {
			return fmt.Errorf("delete archive object: %w", err)
		}
	}

	if err := s.repo.DeleteArchive(ctx, contentdb.DeleteArchiveParams{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
	}); err != nil {
		return fmt.Errorf("delete archive: %w", err)
	}

	return nil
}

func (s *ContentService) getArchive(ctx context.Context, workspaceID, archiveID string) (contentdb.WorkspaceArchive, error) {
	var wID, aID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return contentdb.WorkspaceArchive{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := aID.Scan(archiveID); err != nil {
		return contentdb.WorkspaceArchive{}, ErrArchiveNotFound
	}

	row, err := s.repo.GetArchive(ctx, contentdb.GetArchiveParams{ID: aID, WorkspaceID: wID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contentdb.WorkspaceArchive{}, ErrArchiveNotFound
	}
	if err != nil {
		return contentdb.WorkspaceArchive{}, fmt.Errorf("get archive: %w", err)
	}

	return row, nil
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func (s *ContentService) buildArchive(ctx context.Context, row contentdb.WorkspaceArchive, workspaceID, slug string, actor Actor) {
	hasher := sha256.New()
	counter := &countingWriter{}

	pr, pw := io.Pipe()
	putDone := make(chan error, 1)

	go func() {
		putDone <- s.store.Put(ctx, row.ObjectKey, pr, -1, "application/zip")
	}()

	zw := zip.NewWriter(io.MultiWriter(pw, hasher, counter))
	summary, err := s.writeArchiveContents(ctx, zw, workspaceID, slug, row, actor)
	if err == nil {
		err = zw.Close()
	}

	// Menutup pipe dengan error membatalkan Put; multipart yang menggantung
	// disapu AbortIncompleteUploads dalam 24 jam.
	_ = pw.CloseWithError(err)

	if putErr := <-putDone; err == nil {
		err = putErr
	}

	if err != nil {
		log.Printf("archive %s: %v", row.ID.String(), err)
		if markErr := s.repo.MarkArchiveFailed(ctx, contentdb.MarkArchiveFailedParams{
			ID:    row.ID,
			Error: truncateBytes(err.Error(), 500),
		}); markErr != nil {
			log.Printf("archive %s: mark failed: %v", row.ID.String(), markErr)
		}
		return
	}

	if err := s.repo.MarkArchiveReady(ctx, contentdb.MarkArchiveReadyParams{
		ID:             row.ID,
		SizeBytes:      counter.n,
		ChecksumSha256: hex.EncodeToString(hasher.Sum(nil)),
		DocumentCount:  int32(summary.documents),
		MissingCount:   int32(summary.missing),
	}); err != nil {
		log.Printf("archive %s: mark ready: %v", row.ID.String(), err)
		return
	}

	meta := archiveScopeMetadata(row)
	meta["document_count"] = summary.documents
	meta["missing_count"] = summary.missing
	meta["size_bytes"] = counter.n

	s.activity.Record(ctx, activityservice.NewEntry(workspaceID, actor.UserID, actor.Name, actor.Role,
		activityservice.ActionArchiveExported, activityservice.TargetArchive,
		row.ID.String(), archiveRootName(slug, row.CreatedAt.Time, row.ScopeFolderNames), meta))
}

type archiveSummary struct {
	documents int
	missing   int
}

type archiveIndexRow struct {
	number       string
	zipPath      string
	originalPath string
	name         string
	versionNo    int32
	uploadedBy   string
	uploadedAt   time.Time
	pageCount    int32
	size         int64
	sha256       string
	status       string
}

func csvWriteAll(w io.Writer, header []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		if err := cw.Write(r); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func addZipEntry(zw *zip.Writer, name string, method uint16, modified time.Time) (io.Writer, error) {
	return zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   method,
		Modified: modified,
	})
}

type archiveNode struct {
	folders []contentdb.ListArchiveFoldersRow
	docs    []contentdb.ListArchiveDocumentsRow
}

func (s *ContentService) writeArchiveContents(
	ctx context.Context,
	zw *zip.Writer,
	workspaceID, slug string,
	row contentdb.WorkspaceArchive,
	actor Actor,
) (archiveSummary, error) {
	var summary archiveSummary

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return summary, fmt.Errorf("parse workspace id: %w", err)
	}

	folders, err := s.repo.ListArchiveFolders(ctx, wID)
	if err != nil {
		return summary, fmt.Errorf("list archive folders: %w", err)
	}

	docs, err := s.repo.ListArchiveDocuments(ctx, wID)
	if err != nil {
		return summary, fmt.Errorf("list archive documents: %w", err)
	}

	tree := map[string]*archiveNode{}
	node := func(key string) *archiveNode {
		n, ok := tree[key]
		if !ok {
			n = &archiveNode{}
			tree[key] = n
		}
		return n
	}

	for _, f := range folders {
		node(f.ParentID.String()).folders = append(node(f.ParentID.String()).folders, f)
	}
	for _, d := range docs {
		node(d.FolderID.String()).docs = append(node(d.FolderID.String()).docs, d)
	}

	createdAt := row.CreatedAt.Time
	root := archiveRootName(slug, createdAt, row.ScopeFolderNames)
	docsRoot := root + "/" + archiveDocumentsDir

	// scope nil = seluruh ruang. Paket berlingkup tetap menelusuri pohon utuh:
	// nomor dan sufiks dedup dihitung atas SEMUA saudara supaya identik dengan
	// paket seluruh ruang, dan hanya node dalam cakupan yang ditulis — nomor
	// yang melompat menandai folder di luar cakupan, bukan berkas yang hilang.
	scope := archiveScopeSet(row.ScopeFolderIds)
	seen := make(map[string]struct{}, len(scope))

	index := make([]archiveIndexRow, 0, len(docs))
	folderPaths := map[string]string{}
	usedPaths := map[string]struct{}{}

	var walk func(parentKey, numberPrefix string, dirs, dirsPlain []string, included bool) error
	walk = func(parentKey, numberPrefix string, dirs, dirsPlain []string, included bool) error {
		n, ok := tree[parentKey]
		if !ok {
			return nil
		}

		siblings := len(n.folders) + len(n.docs)
		usedHere := map[string]struct{}{}
		seq := 0

		for _, f := range n.folders {
			seq++
			number := archiveNumber(numberPrefix, seq, siblings)
			plain := dedupName(usedHere, sanitizeComponent(f.Name))

			fid := f.ID.String()
			childIncluded := included
			if _, chosen := scope[fid]; chosen {
				childIncluded = true
				seen[fid] = struct{}{}
			}

			if childIncluded {
				folderPaths[fid] = strings.Join(append(append([]string{}, dirsPlain...), plain), " / ")
			}

			if err := walk(
				fid,
				number,
				append(append([]string{}, dirs...), number+" "+plain),
				append(append([]string{}, dirsPlain...), plain),
				childIncluded,
			); err != nil {
				return err
			}
		}

		for _, d := range n.docs {
			seq++
			number := archiveNumber(numberPrefix, seq, siblings)
			plain := dedupName(usedHere, sanitizeFileName(d.Name))

			// Di luar cakupan: nomor dan nama sudah dihitung, tidak ada I/O.
			if !included {
				continue
			}

			entry, err := s.writeArchiveDocument(ctx, zw, workspaceID, d, archivePath{
				root:      docsRoot,
				dirs:      dirs,
				dirsPlain: dirsPlain,
				file:      number + " " + plain,
				filePlain: plain,
			}, number, usedPaths)
			if err != nil {
				return err
			}

			if entry.status == "ok" {
				summary.documents++
			} else {
				summary.missing++
			}

			index = append(index, entry)
		}

		return nil
	}

	if err := walk("", "", nil, nil, scope == nil); err != nil {
		return summary, err
	}

	if err := s.writeArchiveIndexes(zw, root, index, createdAt); err != nil {
		return summary, err
	}

	// Jejak audit bersifat seluruh ruang — nama dokumen di luar cakupan, semua
	// grup dan anggota — jadi hanya paket seluruh ruang yang membawanya. Paket
	// berlingkup paling mungkin diserahkan ke pihak ketiga; BACA-DULU
	// menjelaskan ketiadaannya.
	if scope == nil {
		if err := s.writeArchiveAudit(ctx, zw, root, workspaceID, wID, folderPaths, actor, createdAt); err != nil {
			return summary, err
		}
	}

	if err := s.writeArchiveReadme(zw, root, row, summary, createdAt, archiveScopeMissing(row, seen)); err != nil {
		return summary, err
	}

	return summary, nil
}

func (s *ContentService) writeArchiveDocument(
	ctx context.Context,
	zw *zip.Writer,
	workspaceID string,
	d contentdb.ListArchiveDocumentsRow,
	p archivePath,
	number string,
	usedPaths map[string]struct{},
) (archiveIndexRow, error) {
	entry := archiveIndexRow{
		number:       number,
		originalPath: joinPath("", p.dirsPlain, d.Name),
		name:         d.Name,
		versionNo:    d.VersionNo,
		uploadedBy:   d.UploadedByName,
		uploadedAt:   d.VersionCreatedAt.Time,
		size:         d.Size,
		status:       "ok",
	}
	if d.PageCount != nil {
		entry.pageCount = *d.PageCount
	}

	key, err := archiveRenditionKey(d)
	if err != nil {
		entry.status = archiveMissingStatus(err)
		return entry, nil
	}

	zipPath, relocated := resolveArchivePath(p)
	if relocated {
		entry.status = "dipindahkan: path terlalu panjang"
	}

	dir, base := splitZipPath(zipPath)
	zipPath = dir + dedupName(usedPaths, base)
	entry.zipPath = zipPath

	w, err := addZipEntry(zw, zipPath, zip.Store, entry.uploadedAt)
	if err != nil {
		return entry, fmt.Errorf("zip entry %s: %w", zipPath, err)
	}

	rc, err := s.renditionGet(ctx, key)
	if err != nil {
		entry.status = "hilang: rendition tidak terbaca"
		return entry, nil
	}
	defer rc.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), rc)
	if err != nil {
		return entry, fmt.Errorf("copy rendition %s: %w", zipPath, err)
	}

	entry.size = n
	entry.sha256 = hex.EncodeToString(h.Sum(nil))

	return entry, nil
}

// archiveRenditionKey reports the clean rendition key for one row of the
// archive listing. It never builds one: CreateArchive wakes the rendition
// worker instead, and a document still converting is listed as not ready,
// the same way a failed one is listed — reported, never silently dropped.
func archiveRenditionKey(d contentdb.ListArchiveDocumentsRow) (string, error) {
	if d.RenditionFailedAt.Valid {
		return "", ErrRenditionFailed
	}

	if d.RenditionKey != nil && *d.RenditionKey != "" {
		return *d.RenditionKey, nil
	}

	return "", ErrRenditionPending
}

func archiveMissingStatus(err error) string {
	if errors.Is(err, ErrRenditionPending) {
		return "belum siap: sedang disiapkan"
	}

	return "hilang: " + truncateBytes(err.Error(), 160)
}

func splitZipPath(p string) (dir, base string) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", p
	}
	return p[:i+1], p[i+1:]
}

func (s *ContentService) writeArchiveIndexes(zw *zip.Writer, root string, index []archiveIndexRow, at time.Time) error {
	rows := make([][]string, 0, len(index))
	for _, e := range index {
		rows = append(rows, []string{
			e.number,
			e.zipPath,
			e.name,
			strconv.FormatInt(int64(e.versionNo), 10),
			e.uploadedBy,
			e.uploadedAt.Format(time.RFC3339),
			strconv.FormatInt(int64(e.pageCount), 10),
			strconv.FormatInt(e.size, 10),
			e.sha256,
			e.status,
			e.originalPath,
		})
	}

	w, err := addZipEntry(zw, root+"/_indeks.csv", zip.Deflate, at)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return err
	}
	if err := csvWriteAll(w, []string{
		"nomor", "path", "nama", "versi", "pengunggah", "tanggal",
		"halaman", "ukuran", "sha256", "status", "path_asli",
	}, rows); err != nil {
		return err
	}

	h, err := addZipEntry(zw, root+"/_indeks.html", zip.Deflate, at)
	if err != nil {
		return err
	}

	return writeArchiveIndexHTML(h, root, index, at)
}

func writeArchiveIndexHTML(w io.Writer, root string, index []archiveIndexRow, at time.Time) error {
	var b strings.Builder

	b.WriteString(`<!doctype html><html lang="id"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>Indeks arsip ` + htmlEscape(root) + `</title><style>`)
	b.WriteString(`:root{color-scheme:light dark}`)
	b.WriteString(`body{font:14px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;margin:2rem auto;max-width:70rem;padding:0 1rem}`)
	b.WriteString(`h1{font-size:1.25rem;margin:0 0 .25rem}`)
	b.WriteString(`p.meta{color:#666;margin:0 0 1.5rem}`)
	b.WriteString(`table{border-collapse:collapse;width:100%}`)
	b.WriteString(`th,td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #8883;vertical-align:top}`)
	b.WriteString(`th{font-weight:600;white-space:nowrap}`)
	b.WriteString(`td.num,td.hash,td.size{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap}`)
	b.WriteString(`td.hash{font-size:.75rem;color:#666}`)
	b.WriteString(`tr.missing td{color:#a33}`)
	b.WriteString(`</style></head><body>`)
	b.WriteString(`<h1>` + htmlEscape(root) + `</h1>`)
	b.WriteString(`<p class="meta">Dibuat ` + htmlEscape(at.Format("2 January 2006 15:04 MST")) +
		` · ` + strconv.Itoa(len(index)) + ` dokumen</p>`)
	b.WriteString(`<table><thead><tr><th>Nomor</th><th>Dokumen</th><th>Versi</th>` +
		`<th>Pengunggah</th><th>Tanggal</th><th>Halaman</th><th>Ukuran</th><th>SHA-256</th><th>Status</th>` +
		`</tr></thead><tbody>`)

	for _, e := range index {
		cls := ""
		if e.status != "ok" {
			cls = ` class="missing"`
		}

		b.WriteString(`<tr` + cls + `><td class="num">` + htmlEscape(e.number) + `</td><td>`)
		if e.zipPath != "" && e.status == "ok" {
			b.WriteString(`<a href="` + htmlEscape(relativeFromRoot(root, e.zipPath)) + `">` + htmlEscape(e.name) + `</a>`)
		} else {
			b.WriteString(htmlEscape(e.name))
		}
		b.WriteString(`</td><td class="num">` + strconv.FormatInt(int64(e.versionNo), 10) + `</td>`)
		b.WriteString(`<td>` + htmlEscape(e.uploadedBy) + `</td>`)
		b.WriteString(`<td class="num">` + htmlEscape(e.uploadedAt.Format(time.DateOnly)) + `</td>`)
		b.WriteString(`<td class="num">` + strconv.FormatInt(int64(e.pageCount), 10) + `</td>`)
		b.WriteString(`<td class="size">` + strconv.FormatInt(e.size, 10) + `</td>`)
		b.WriteString(`<td class="hash">` + htmlEscape(e.sha256) + `</td>`)
		b.WriteString(`<td>` + htmlEscape(e.status) + `</td></tr>`)
	}

	b.WriteString(`</tbody></table></body></html>`)

	_, err := io.WriteString(w, b.String())
	return err
}

func relativeFromRoot(root, zipPath string) string {
	return strings.TrimPrefix(zipPath, root+"/")
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;",
)

func htmlEscape(s string) string { return htmlEscaper.Replace(s) }

func (s *ContentService) writeArchiveAudit(
	ctx context.Context,
	zw *zip.Writer,
	root, workspaceID string,
	wID pgtype.UUID,
	folderPaths map[string]string,
	actor Actor,
	at time.Time,
) error {
	if s.activityExport != nil {
		w, err := addZipEntry(zw, root+"/_audit/aktivitas.csv", zip.Deflate, at)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
			return err
		}
		if err := s.activityExport.ExportActivityCSV(ctx, w, workspaceID, actor.Role); err != nil {
			return fmt.Errorf("export activity csv: %w", err)
		}
	}

	if s.qaExport != nil {
		w, err := addZipEntry(zw, root+"/_audit/qa.csv", zip.Deflate, at)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
			return err
		}
		if err := s.qaExport.ExportQuestionsCSV(ctx, w, workspaceID, actor.UserID, actor.Name, actor.Email, actor.Role); err != nil {
			return fmt.Errorf("export questions csv: %w", err)
		}
	}

	matrix, err := s.repo.ListFolderAccessMatrix(ctx, wID)
	if err != nil {
		return fmt.Errorf("list folder access matrix: %w", err)
	}

	rows := make([][]string, 0, len(matrix))
	for _, m := range matrix {
		folderID := m.FolderID.String()
		source := m.SourceFolderID.String()
		inherited := ""
		if source != folderID {
			inherited = folderPaths[source]
		}

		rows = append(rows, []string{
			folderPaths[folderID],
			m.GroupName,
			strconv.FormatBool(m.CanView),
			strconv.FormatBool(m.CanDownload),
			strconv.FormatBool(m.CanWatermark),
			strconv.FormatBool(m.CanDownloadOriginal),
			inherited,
		})
	}

	w, err := addZipEntry(zw, root+"/_audit/izin.csv", zip.Deflate, at)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return err
	}
	if err := csvWriteAll(w, []string{
		"folder", "grup", "can_view", "can_download", "can_watermark",
		"can_download_original", "diwarisi_dari",
	}, rows); err != nil {
		return err
	}

	members, err := s.repo.ListArchiveMembers(ctx, wID)
	if err != nil {
		return fmt.Errorf("list archive members: %w", err)
	}

	memberRows := make([][]string, 0, len(members))
	for _, m := range members {
		memberRows = append(memberRows, []string{
			m.Username,
			m.Email,
			m.RoleName,
			m.Status,
			strings.Join(m.GroupNames, "; "),
			m.CreatedAt.Time.Format(time.RFC3339),
		})
	}

	mw, err := addZipEntry(zw, root+"/_audit/anggota.csv", zip.Deflate, at)
	if err != nil {
		return err
	}
	if _, err := mw.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return err
	}

	return csvWriteAll(mw, []string{
		"username", "email", "peran", "status", "grup", "bergabung",
	}, memberRows)
}

func (s *ContentService) writeArchiveReadme(
	zw *zip.Writer,
	root string,
	row contentdb.WorkspaceArchive,
	summary archiveSummary,
	at time.Time,
	missingScope []string,
) error {
	w, err := addZipEntry(zw, root+"/BACA-DULU.txt", zip.Deflate, at)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, archiveReadmeText(root, row, summary, at, missingScope))
	return err
}

// archiveReadmeText adalah salinan BACA-DULU.txt, dipisah dari penulisan ZIP
// supaya bisa diuji tanpa merakit arsip. Teks paket seluruh ruang tidak
// berubah; paket berlingkup menyebut cakupannya dan menjelaskan mengapa tidak
// ada _audit/.
func archiveReadmeText(root string, row contentdb.WorkspaceArchive, summary archiveSummary, at time.Time, missingScope []string) string {
	scoped := len(row.ScopeFolderNames) > 0

	var b strings.Builder
	b.WriteString("Arsip ruang data " + root + "\r\n")
	b.WriteString("Dibuat " + at.Format(time.RFC3339) + " oleh " + row.RequestedByName + "\r\n")
	if scoped {
		b.WriteString("Cakupan paket: " + strconv.Itoa(len(row.ScopeFolderNames)) + " folder (bukan seluruh ruang)\r\n")
		for _, name := range row.ScopeFolderNames {
			b.WriteString("  - " + name + "\r\n")
		}
		if len(missingScope) > 0 {
			b.WriteString("Folder cakupan yang sudah tidak ada saat perakitan: " + strings.Join(missingScope, ", ") + "\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString("Isi paket\r\n")
	if scoped {
		b.WriteString("  dokumen/       PDF rendition setiap dokumen dalam cakupan, tanpa watermark.\r\n")
	} else {
		b.WriteString("  dokumen/       PDF rendition setiap dokumen, tanpa watermark.\r\n")
	}
	b.WriteString("  _indeks.html   Daftar dokumen yang bisa diklik.\r\n")
	b.WriteString("  _indeks.csv    Daftar yang sama untuk diproses mesin.\r\n")
	if scoped {
		b.WriteString("  (tanpa _audit/) Jejak audit — linimasa aktivitas, riwayat Q&A, matriks izin,\r\n")
		b.WriteString("                 daftar anggota — hanya ada di paket seluruh ruang.\r\n")
	} else {
		b.WriteString("  _audit/        Linimasa aktivitas, riwayat Q&A, matriks izin, daftar anggota.\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString("Dokumen: " + strconv.Itoa(summary.documents) + " disertakan, " +
		strconv.Itoa(summary.missing) + " tidak disertakan.\r\n")
	b.WriteString("\r\n")
	b.WriteString("Yang TIDAK dijanjikan paket ini\r\n")
	b.WriteString("  Isi paket adalah keadaan ruang pada stempel waktu ekspor di atas,\r\n")
	b.WriteString("  bukan pada detik ruang diarsipkan.\r\n")
	b.WriteString("  Hanya versi berjalan yang disertakan. Riwayat versi tidak ikut.\r\n")
	b.WriteString("  Dokumen yang konversinya gagal tercatat di indeks dengan status\r\n")
	b.WriteString("  hilang, yang masih dikonversi dengan status belum siap; keduanya\r\n")
	b.WriteString("  tidak ada berkasnya di folder dokumen. Ekspor ulang setelah\r\n")
	b.WriteString("  konversi selesai untuk menyertakannya.\r\n")
	if scoped {
		b.WriteString("  Nomor pada nama berkas sama dengan nomor di paket seluruh ruang;\r\n")
		b.WriteString("  nomor yang melompat menandai folder di luar cakupan, bukan berkas\r\n")
		b.WriteString("  yang hilang.\r\n")
	} else {
		b.WriteString("  Nomor pada nama berkas dihitung ulang untuk seluruh ruang, jadi\r\n")
		b.WriteString("  bisa berbeda dari nomor yang pernah dilihat seorang tamu.\r\n")
	}
	b.WriteString("  Berkas yang path-nya terlalu panjang dipindahkan ke folder\r\n")
	b.WriteString("  " + relocatedDir + "/ dengan path asli tercatat di indeks.\r\n")

	return b.String()
}

// RunArchiveSweeper adalah sweeper keenam, bentuk kanonik RunReaper. Tiga tugas:
// hapus objek arsip yang lewat TTL, tandai baris pending yang basi sebagai
// failed, dan hapus baris yang sudah kedaluwarsa.
func (s *ContentService) RunArchiveSweeper(ctx context.Context, interval, ttl time.Duration) {
	s.sweepArchivesOnce(ctx, ttl)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepArchivesOnce(ctx, ttl)
		}
	}
}

func (s *ContentService) sweepArchivesOnce(ctx context.Context, ttl time.Duration) {
	deleted, err := s.store.DeleteOlderThan(ctx, "archives/", ttl)
	if err != nil {
		log.Printf("archive sweep: delete objects: %v", err)
	}

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-archiveStalePendingAge), Valid: true}
	stale, err := s.repo.ListStalePendingArchives(ctx, cutoff)
	if err != nil {
		log.Printf("archive sweep: list stale pending: %v", err)
	}

	for _, a := range stale {
		if err := s.repo.MarkArchiveFailed(ctx, contentdb.MarkArchiveFailedParams{
			ID:    a.ID,
			Error: "pembuatan arsip terhenti sebelum selesai",
		}); err != nil {
			log.Printf("archive sweep: mark stale %s: %v", a.ID.String(), err)
		}
	}

	expired, err := s.repo.ListExpiredArchives(ctx)
	if err != nil {
		log.Printf("archive sweep: list expired: %v", err)
		return
	}

	purged := 0
	for _, a := range expired {
		if a.ObjectKey != "" {
			if err := s.store.Delete(ctx, a.ObjectKey); err != nil {
				log.Printf("archive sweep: delete %s: %v", a.ObjectKey, err)
				continue
			}
		}

		if err := s.repo.DeleteArchive(ctx, contentdb.DeleteArchiveParams{
			ID:          a.ID,
			WorkspaceID: a.WorkspaceID,
		}); err != nil {
			log.Printf("archive sweep: delete row %s: %v", a.ID.String(), err)
			continue
		}

		purged++
	}

	if deleted > 0 || purged > 0 || len(stale) > 0 {
		log.Printf("archive sweep: deleted %d objects, purged %d rows, failed %d stale", deleted, purged, len(stale))
	}
}
