package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture ruangan untuk uji arsip. Nomor mengikuti archiveNumber atas SEMUA
// saudara (folder + dokumen) per induk, jadi:
//
//	01 General            (default, kosong)
//	02 Korporasi
//	  02.01 Akta            → 02.01.01 akta.pdf   (rendition siap)
//	  02.02 Anggaran Dasar  → 02.02.01 ad.pdf     (rendition belum ada)
//	03 Keuangan           → 03.01 laporan.pdf   (rendition siap)
const (
	archiveTestSlug = "ruang-1a2b3c4d"
	archiveTestWS   = "00000000-0000-0000-0000-0000000000aa"
)

var archiveTestDate = time.Date(2026, 9, 4, 9, 30, 0, 0, time.UTC)

func archiveUUID(n byte) pgtype.UUID {
	var b [16]byte
	b[15] = n
	return pgtype.UUID{Bytes: b, Valid: true}
}

var (
	folderGeneral   = archiveUUID(1)
	folderKorporasi = archiveUUID(2)
	folderAkta      = archiveUUID(3)
	folderAD        = archiveUUID(4)
	folderKeuangan  = archiveUUID(5)
	folderUnknown   = archiveUUID(9)
)

func archiveTestFolders() []contentdb.ListArchiveFoldersRow {
	return []contentdb.ListArchiveFoldersRow{
		{ID: folderGeneral, Name: "General", Position: 0, IsDefault: true},
		{ID: folderKorporasi, Name: "Korporasi", Position: 1},
		{ID: folderKeuangan, Name: "Keuangan", Position: 2},
		{ID: folderAkta, ParentID: folderKorporasi, Name: "Akta", Position: 0},
		{ID: folderAD, ParentID: folderKorporasi, Name: "Anggaran Dasar", Position: 1},
	}
}

func archiveTestDocs() []contentdb.ListArchiveDocumentsRow {
	akta := "r/akta.pdf"
	laporan := "r/laporan.pdf"
	at := pgtype.Timestamptz{Time: archiveTestDate.Add(-time.Hour), Valid: true}
	return []contentdb.ListArchiveDocumentsRow{
		{ID: archiveUUID(21), FolderID: folderAkta, Name: "akta.pdf", VersionNo: 1, RenditionKey: &akta, UploadedByName: "Ardi", VersionCreatedAt: at},
		{ID: archiveUUID(22), FolderID: folderAD, Name: "ad.pdf", VersionNo: 1, UploadedByName: "Ardi", VersionCreatedAt: at},
		{ID: archiveUUID(23), FolderID: folderKeuangan, Name: "laporan.pdf", VersionNo: 2, RenditionKey: &laporan, UploadedByName: "Ardi", VersionCreatedAt: at},
	}
}

type archiveFakeRepo struct {
	ContentRepository

	folders []contentdb.ListArchiveFoldersRow
	docs    []contentdb.ListArchiveDocumentsRow
	pending int32

	created     []contentdb.CreateArchiveParams
	listFolders atomic.Int32
	matrixCalls atomic.Int32
	memberCalls atomic.Int32
	ready       chan contentdb.MarkArchiveReadyParams
	failed      chan contentdb.MarkArchiveFailedParams
}

func newArchiveFakeRepo() *archiveFakeRepo {
	return &archiveFakeRepo{
		folders: archiveTestFolders(),
		docs:    archiveTestDocs(),
		ready:   make(chan contentdb.MarkArchiveReadyParams, 4),
		failed:  make(chan contentdb.MarkArchiveFailedParams, 4),
	}
}

func (f *archiveFakeRepo) GetWorkspaceSlugForArchive(context.Context, pgtype.UUID) (string, error) {
	return archiveTestSlug, nil
}

func (f *archiveFakeRepo) CountPendingArchives(context.Context, pgtype.UUID) (int32, error) {
	return f.pending, nil
}

func (f *archiveFakeRepo) CreateArchive(_ context.Context, p contentdb.CreateArchiveParams) (contentdb.WorkspaceArchive, error) {
	f.created = append(f.created, p)
	return contentdb.WorkspaceArchive{
		ID:               archiveUUID(100),
		WorkspaceID:      p.WorkspaceID,
		RequestedBy:      p.RequestedBy,
		RequestedByName:  p.RequestedByName,
		Status:           ArchiveStatusPending,
		CreatedAt:        pgtype.Timestamptz{Time: archiveTestDate, Valid: true},
		ExpiresAt:        p.ExpiresAt,
		ScopeFolderIds:   p.ScopeFolderIds,
		ScopeFolderNames: p.ScopeFolderNames,
	}, nil
}

func (f *archiveFakeRepo) SetArchiveObjectKey(context.Context, contentdb.SetArchiveObjectKeyParams) error {
	return nil
}

func (f *archiveFakeRepo) ListArchiveFolders(context.Context, pgtype.UUID) ([]contentdb.ListArchiveFoldersRow, error) {
	f.listFolders.Add(1)
	return f.folders, nil
}

func (f *archiveFakeRepo) ListArchiveDocuments(context.Context, pgtype.UUID) ([]contentdb.ListArchiveDocumentsRow, error) {
	return f.docs, nil
}

func (f *archiveFakeRepo) MarkArchiveReady(_ context.Context, p contentdb.MarkArchiveReadyParams) error {
	f.ready <- p
	return nil
}

func (f *archiveFakeRepo) MarkArchiveFailed(_ context.Context, p contentdb.MarkArchiveFailedParams) error {
	f.failed <- p
	return nil
}

func (f *archiveFakeRepo) ListFolderAccessMatrix(context.Context, pgtype.UUID) ([]contentdb.ListFolderAccessMatrixRow, error) {
	f.matrixCalls.Add(1)
	return []contentdb.ListFolderAccessMatrixRow{
		{FolderID: folderKeuangan, GroupName: "Investor", CanView: true, SourceFolderID: folderKeuangan},
	}, nil
}

func (f *archiveFakeRepo) ListArchiveMembers(context.Context, pgtype.UUID) ([]contentdb.ListArchiveMembersRow, error) {
	f.memberCalls.Add(1)
	return []contentdb.ListArchiveMembersRow{
		{Username: "ardi", Email: "ardi@example.com", RoleName: "owner", Status: "active", CreatedAt: pgtype.Timestamptz{Time: archiveTestDate, Valid: true}},
	}, nil
}

// channelActivity dipakai alih-alih recordingActivity karena Record dipanggil
// dari goroutine perakitan; membaca slice setelahnya adalah data race.
type channelActivity struct{ ch chan activityservice.Entry }

func newChannelActivity() *channelActivity {
	return &channelActivity{ch: make(chan activityservice.Entry, 4)}
}

func (c *channelActivity) Record(_ context.Context, e activityservice.Entry) { c.ch <- e }

func (c *channelActivity) RecordTx(_ context.Context, _ pgx.Tx, e activityservice.Entry) error {
	c.ch <- e
	return nil
}

func (c *channelActivity) RecordPageEvent(context.Context, activityservice.PageEvent) {}

type stubExporter struct{}

func (stubExporter) ExportActivityCSV(_ context.Context, w io.Writer, _, _ string) error {
	_, err := io.WriteString(w, "waktu,aksi\n")
	return err
}

func (stubExporter) ExportQuestionsCSV(_ context.Context, w io.Writer, _, _, _, _, _ string) error {
	_, err := io.WriteString(w, "nomor,pertanyaan\n")
	return err
}

func newArchiveTestService(t *testing.T, repo *archiveFakeRepo, store *cacheFakeStorage, act ActivityRecorder) *ContentService {
	t.Helper()
	return NewContentService(repo, store, Viewer{}, 0, act, StampDeps{},
		ArchiveDeps{Concurrency: 1, ActivityExport: stubExporter{}, QAExport: stubExporter{}},
		CacheDeps{}, RenditionDeps{})
}

func newArchiveTestStorage() *cacheFakeStorage {
	store := newCacheFakeStorage()
	store.objects["r/akta.pdf"] = []byte("%PDF akta")
	store.objects["r/laporan.pdf"] = []byte("%PDF laporan")
	return store
}

func archiveTestActor() Actor {
	return Actor{UserID: "00000000-0000-0000-0000-0000000000bb", Role: permission.RoleOwner, Name: "Ardi"}
}

func archiveTestRow(scopeIDs []pgtype.UUID, scopeNames []string) contentdb.WorkspaceArchive {
	return contentdb.WorkspaceArchive{
		ID:               archiveUUID(100),
		RequestedByName:  "Ardi",
		Status:           ArchiveStatusPending,
		ObjectKey:        archiveObjectKey(archiveTestWS, uuidString(archiveUUID(100))),
		CreatedAt:        pgtype.Timestamptz{Time: archiveTestDate, Valid: true},
		ScopeFolderIds:   scopeIDs,
		ScopeFolderNames: scopeNames,
	}
}

func zipEntries(t *testing.T, b []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)

	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func zipFile(t *testing.T, b []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)

	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		defer rc.Close()
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		return string(body)
	}

	t.Fatalf("entry %q not in zip", name)
	return ""
}

// buildAndRead merakit satu paket secara sinkron dan mengembalikan byte ZIP-nya
// bersama parameter MarkArchiveReady dan baris aktivitas yang ditulis.
func buildAndRead(t *testing.T, repo *archiveFakeRepo, row contentdb.WorkspaceArchive) ([]byte, contentdb.MarkArchiveReadyParams, activityservice.Entry) {
	t.Helper()
	store := newArchiveTestStorage()
	act := newChannelActivity()
	svc := newArchiveTestService(t, repo, store, act)

	svc.buildArchive(context.Background(), row, archiveTestWS, archiveTestSlug, archiveTestActor())

	select {
	case p := <-repo.failed:
		t.Fatalf("archive failed: %s", p.Error)
	default:
	}

	ready := recvOrFatal(t, repo.ready, "MarkArchiveReady")
	entry := recvOrFatal(t, act.ch, "activity entry")

	store.mu.Lock()
	b, ok := store.objects[row.ObjectKey]
	store.mu.Unlock()
	require.True(t, ok, "zip stored under object key")
	return b, ready, entry
}

func TestCreateArchiveScopeUnknownFolderIsNotFound(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
	}{
		{name: "missing id", ids: []string{uuidString(folderUnknown)}},
		{name: "one bad among good", ids: []string{uuidString(folderKeuangan), uuidString(folderUnknown)}},
		{name: "not a uuid", ids: []string{"bukan-uuid"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newArchiveFakeRepo()
			svc := newArchiveTestService(t, repo, newArchiveTestStorage(), newChannelActivity())

			_, err := svc.CreateArchive(context.Background(), archiveTestWS, archiveTestActor(), dto.CreateArchiveRequest{FolderIDs: tc.ids})

			require.ErrorIs(t, err, ErrFolderNotFound)
			assert.Empty(t, repo.created, "validate-all-first: nothing inserted")
			assert.Len(t, svc.archiveSem, 0, "no semaphore slot taken")
		})
	}
}

func TestCreateArchiveScopeDedupesAndSnapshotsNames(t *testing.T) {
	repo := newArchiveFakeRepo()
	act := newChannelActivity()
	svc := newArchiveTestService(t, repo, newArchiveTestStorage(), act)

	res, err := svc.CreateArchive(context.Background(), archiveTestWS, archiveTestActor(), dto.CreateArchiveRequest{
		FolderIDs: []string{uuidString(folderKorporasi), uuidString(folderKeuangan), uuidString(folderKorporasi)},
	})
	require.NoError(t, err)
	recvOrFatal(t, act.ch, "build to finish")

	require.Len(t, repo.created, 1)
	assert.Equal(t, []pgtype.UUID{folderKorporasi, folderKeuangan}, repo.created[0].ScopeFolderIds)
	assert.Equal(t, []string{"Korporasi", "Keuangan"}, repo.created[0].ScopeFolderNames)

	assert.Equal(t, dto.ArchiveScopeFolders, res.Scope)
	assert.Equal(t, []string{uuidString(folderKorporasi), uuidString(folderKeuangan)}, res.ScopeFolderIDs)
	assert.Equal(t, []string{"Korporasi", "Keuangan"}, res.ScopeFolderNames)
}

func TestCreateArchiveWholeRoomPassesNullScope(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
	}{
		{name: "nil", ids: nil},
		{name: "empty", ids: []string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newArchiveFakeRepo()
			act := newChannelActivity()
			svc := newArchiveTestService(t, repo, newArchiveTestStorage(), act)

			res, err := svc.CreateArchive(context.Background(), archiveTestWS, archiveTestActor(), dto.CreateArchiveRequest{FolderIDs: tc.ids})
			require.NoError(t, err)
			recvOrFatal(t, act.ch, "build to finish")

			require.Len(t, repo.created, 1)
			assert.Nil(t, repo.created[0].ScopeFolderIds, "nil slice → SQL NULL, never '{}'")
			assert.Nil(t, repo.created[0].ScopeFolderNames)
			assert.Equal(t, dto.ArchiveScopeRoom, res.Scope)
			assert.Nil(t, res.ScopeFolderIDs)
			assert.Nil(t, res.ScopeFolderNames)
		})
	}
}

func TestResolveArchiveScopeWholeRoomSkipsFolderQuery(t *testing.T) {
	repo := newArchiveFakeRepo()
	svc := newArchiveTestService(t, repo, newArchiveTestStorage(), newChannelActivity())

	scope, err := svc.resolveArchiveScope(context.Background(), archiveUUID(0xaa), nil)

	require.NoError(t, err)
	assert.Nil(t, scope.ids)
	assert.Nil(t, scope.names)
	assert.Equal(t, int32(0), repo.listFolders.Load())
}

func TestCreateArchiveScopedSharesOnePendingSlot(t *testing.T) {
	repo := newArchiveFakeRepo()
	repo.pending = 1
	svc := newArchiveTestService(t, repo, newArchiveTestStorage(), newChannelActivity())

	_, err := svc.CreateArchive(context.Background(), archiveTestWS, archiveTestActor(), dto.CreateArchiveRequest{
		FolderIDs: []string{uuidString(folderKeuangan)},
	})

	require.ErrorIs(t, err, ErrArchiveAlreadyQueued)
	assert.Empty(t, repo.created)
}

func TestBuildArchiveWholeRoomLayout(t *testing.T) {
	repo := newArchiveFakeRepo()
	row := archiveTestRow(nil, nil)

	b, ready, entry := buildAndRead(t, repo, row)

	root := "ruang-1a2b3c4d-arsip-2026-09-04"
	assert.Equal(t, []string{
		root + "/BACA-DULU.txt",
		root + "/_audit/aktivitas.csv",
		root + "/_audit/anggota.csv",
		root + "/_audit/izin.csv",
		root + "/_audit/qa.csv",
		root + "/_indeks.csv",
		root + "/_indeks.html",
		root + "/dokumen/02 Korporasi/02.01 Akta/02.01.01 akta.pdf",
		root + "/dokumen/03 Keuangan/03.01 laporan.pdf",
	}, zipEntries(t, b))

	assert.Equal(t, int32(2), ready.DocumentCount)
	assert.Equal(t, int32(1), ready.MissingCount)

	readme := zipFile(t, b, root+"/BACA-DULU.txt")
	assert.Contains(t, readme, "  _audit/        Linimasa aktivitas")
	assert.Contains(t, readme, "dihitung ulang untuk seluruh ruang")
	assert.NotContains(t, readme, "Cakupan paket")

	assert.Equal(t, activityservice.ActionArchiveExported, entry.Action)
	assert.Equal(t, root, entry.TargetName)
	assert.Equal(t, dto.ArchiveScopeRoom, entry.Metadata["scope"])
	assert.NotContains(t, entry.Metadata, "folder_names")
}

func TestBuildArchiveScopedOmitsAuditAndKeepsNumbering(t *testing.T) {
	repo := newArchiveFakeRepo()
	row := archiveTestRow([]pgtype.UUID{folderKeuangan}, []string{"Keuangan"})

	b, ready, entry := buildAndRead(t, repo, row)

	root := "ruang-1a2b3c4d-Keuangan-arsip-2026-09-04"
	assert.Equal(t, []string{
		root + "/BACA-DULU.txt",
		root + "/_indeks.csv",
		root + "/_indeks.html",
		root + "/dokumen/03 Keuangan/03.01 laporan.pdf",
	}, zipEntries(t, b), "numbering keeps the room's 03 even though 01 and 02 are excluded")

	assert.Equal(t, int32(0), repo.matrixCalls.Load(), "no permission matrix read for a scoped package")
	assert.Equal(t, int32(0), repo.memberCalls.Load(), "no member list read for a scoped package")
	assert.Equal(t, int32(1), ready.DocumentCount)
	assert.Equal(t, int32(0), ready.MissingCount)

	index := zipFile(t, b, root+"/_indeks.csv")
	assert.Equal(t, 1, strings.Count(index, "\n")-1, "one data row in the index")
	assert.Contains(t, index, "03.01,")

	assert.Equal(t, root, entry.TargetName)
	assert.Equal(t, dto.ArchiveScopeFolders, entry.Metadata["scope"])
	assert.Equal(t, 1, entry.Metadata["folder_count"])
	assert.Equal(t, []string{"Keuangan"}, entry.Metadata["folder_names"])
}

func TestBuildArchiveScopedNestedFolderKeepsAncestorPath(t *testing.T) {
	repo := newArchiveFakeRepo()
	row := archiveTestRow([]pgtype.UUID{folderAkta}, []string{"Akta"})

	b, ready, _ := buildAndRead(t, repo, row)

	root := "ruang-1a2b3c4d-Akta-arsip-2026-09-04"
	assert.Contains(t, zipEntries(t, b), root+"/dokumen/02 Korporasi/02.01 Akta/02.01.01 akta.pdf")
	assert.NotContains(t, zipEntries(t, b), root+"/dokumen/03 Keuangan/03.01 laporan.pdf")
	assert.Equal(t, int32(1), ready.DocumentCount)
	assert.Equal(t, int32(0), ready.MissingCount, "the pending ad.pdf sits outside the scope and is not counted")
}

func TestBuildArchiveScopedNestedUnderSelectedEmitsOnce(t *testing.T) {
	repo := newArchiveFakeRepo()
	row := archiveTestRow([]pgtype.UUID{folderKorporasi, folderAkta}, []string{"Korporasi", "Akta"})

	b, ready, _ := buildAndRead(t, repo, row)

	root := "ruang-1a2b3c4d-arsip-sebagian-2026-09-04"
	assert.Equal(t, []string{
		root + "/BACA-DULU.txt",
		root + "/_indeks.csv",
		root + "/_indeks.html",
		root + "/dokumen/02 Korporasi/02.01 Akta/02.01.01 akta.pdf",
	}, zipEntries(t, b))
	assert.Equal(t, int32(1), ready.DocumentCount)
	assert.Equal(t, int32(1), ready.MissingCount, "ad.pdf is in scope but has no rendition yet")
}

func TestBuildArchiveScopedReadmeStatesScope(t *testing.T) {
	repo := newArchiveFakeRepo()
	// folderUnknown lolos validasi di masa lalu lalu dihapus: harus disebut.
	row := archiveTestRow(
		[]pgtype.UUID{folderKorporasi, folderKeuangan, folderUnknown},
		[]string{"Korporasi", "Keuangan", "Sudah Dihapus"},
	)

	b, _, _ := buildAndRead(t, repo, row)

	readme := zipFile(t, b, "ruang-1a2b3c4d-arsip-sebagian-2026-09-04/BACA-DULU.txt")
	assert.Contains(t, readme, "Cakupan paket: 3 folder (bukan seluruh ruang)\r\n")
	assert.Contains(t, readme, "  - Korporasi\r\n  - Keuangan\r\n  - Sudah Dihapus\r\n")
	assert.Contains(t, readme, "Folder cakupan yang sudah tidak ada saat perakitan: Sudah Dihapus")
	assert.Contains(t, readme, "hanya ada di paket seluruh ruang")
	assert.Contains(t, readme, "nomor yang melompat menandai folder di luar cakupan")
	assert.NotContains(t, readme, "  _audit/        ")
	assert.NotContains(t, readme, "dihitung ulang untuk seluruh ruang")
	assert.False(t, strings.Contains(strings.ReplaceAll(readme, "\r\n", ""), "\n"), "CRLF only")
}
