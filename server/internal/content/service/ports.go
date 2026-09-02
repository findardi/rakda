package service

import (
	"context"
	"io"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ContentRepository interface {
	CreateFolder(ctx context.Context, arg contentdb.CreateFolderParams) (contentdb.Folder, error)
	GetFolderByID(ctx context.Context, id pgtype.UUID) (contentdb.Folder, error)
	GetFoldersByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.Folder, error)
	GetMaxPositionInParent(ctx context.Context, arg contentdb.GetMaxPositionInParentParams) (int32, error)
	MoveFolder(ctx context.Context, arg contentdb.MoveFolderParams) error
	RenameFolder(ctx context.Context, arg contentdb.RenameFolderParams) (contentdb.Folder, error)
	GetTrashedDocumentByID(ctx context.Context, id pgtype.UUID) (contentdb.Document, error)
	GetTrashedFolderByID(ctx context.Context, id pgtype.UUID) (contentdb.Folder, error)
	GetDefaultFolder(ctx context.Context, workspaceID pgtype.UUID) (contentdb.Folder, error)

	CreateDocument(ctx context.Context, arg contentdb.CreateDocumentParams) (contentdb.Document, error)
	CreateDocumentVersion(ctx context.Context, arg contentdb.CreateDocumentVersionParams) (contentdb.DocumentVersion, error)
	SetCurrentVersion(ctx context.Context, arg contentdb.SetCurrentVersionParams) error
	SetStagedVersion(ctx context.Context, arg contentdb.SetStagedVersionParams) error
	PromoteStagedVersion(ctx context.Context, arg contentdb.PromoteStagedVersionParams) (int64, error)
	GetNextVersionNo(ctx context.Context, documentID pgtype.UUID) (int32, error)
	GetDocumentByID(ctx context.Context, id pgtype.UUID) (contentdb.Document, error)
	GetDocumentByNameInFolder(ctx context.Context, arg contentdb.GetDocumentByNameInFolderParams) (contentdb.Document, error)
	ListDocumentsByFolder(ctx context.Context, folderID pgtype.UUID) ([]contentdb.ListDocumentsByFolderRow, error)
	ListVersionsWithUploader(ctx context.Context, documentID pgtype.UUID) ([]contentdb.ListVersionsWithUploaderRow, error)
	ListVersionByDocument(ctx context.Context, documentID pgtype.UUID) ([]contentdb.DocumentVersion, error)
	ListTrashDocuments(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListTrashDocumentsRow, error)
	ListTrashFolders(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListTrashFoldersRow, error)
	GetVersionByID(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error)
	GetCurrentVersion(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error)
	SetVersionRendition(ctx context.Context, arg contentdb.SetVersionRenditionParams) error
	SetVersionRenditionFailure(ctx context.Context, arg contentdb.SetVersionRenditionFailureParams) error
	ClearVersionRenditionFailure(ctx context.Context, id pgtype.UUID) error
	MoveDocument(ctx context.Context, arg contentdb.MoveDocumentParams) error

	ListFolderAccess(ctx context.Context, arg contentdb.ListFolderAccessParams) ([]contentdb.ListFolderAccessRow, error)
	RemoveFolderAccess(ctx context.Context, arg contentdb.RemoveFolderAccessParams) error
	SetFolderAccess(ctx context.Context, arg contentdb.SetFolderAccessParams) (contentdb.FolderAccess, error)
	ResolveFolderAccess(ctx context.Context, arg contentdb.ResolveFolderAccessParams) (contentdb.ResolveFolderAccessRow, error)
	ListVisibleFolders(ctx context.Context, arg contentdb.ListVisibleFoldersParams) ([]contentdb.ListVisibleFoldersRow, error)

	RestoreDocument(ctx context.Context, arg contentdb.RestoreDocumentParams) error
	RestoreDocumentsSweptBy(ctx context.Context, deletedRootFolderID pgtype.UUID) error
	RestoreFolderRoot(ctx context.Context, arg contentdb.RestoreFolderRootParams) error
	RestoreFoldersSweptBy(ctx context.Context, deletedRootFolderID pgtype.UUID) error

	SoftDeleteDocument(ctx context.Context, arg contentdb.SoftDeleteDocumentParams) error
	SoftDeleteDocumentsForFolderRoot(ctx context.Context, arg contentdb.SoftDeleteDocumentsForFolderRootParams) error
	SoftDeleteFolderSubtree(ctx context.Context, arg contentdb.SoftDeleteFolderSubtreeParams) error

	ListExpiredTrashFolders(ctx context.Context, cutoff pgtype.Timestamptz) ([]contentdb.ListExpiredTrashFoldersRow, error)
	ListExpiredTrashDocuments(ctx context.Context, cutoff pgtype.Timestamptz) ([]contentdb.ListExpiredTrashDocumentsRow, error)
	ListVersionsSweptByFolder(ctx context.Context, folderID pgtype.UUID) ([]contentdb.ListVersionsSweptByFolderRow, error)
	PurgeFolder(ctx context.Context, id pgtype.UUID) error
	PurgeDocument(ctx context.Context, id pgtype.UUID) error

	ListPendingTextExtraction(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error)
	DeleteVersionPageText(ctx context.Context, versionID pgtype.UUID) error
	InsertPageText(ctx context.Context, arg contentdb.InsertPageTextParams) error
	SetVersionTextExtracted(ctx context.Context, id pgtype.UUID) error
	SetVersionTextFailure(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error
	ListPendingOCRPages(ctx context.Context, limitCount int32) ([]contentdb.ListPendingOCRPagesRow, error)
	SetPageOCRResult(ctx context.Context, arg contentdb.SetPageOCRResultParams) error
	SetPageOCRFailure(ctx context.Context, arg contentdb.SetPageOCRFailureParams) error
	ListPendingWordBoxes(ctx context.Context, limitCount int32) ([]contentdb.ListPendingWordBoxesRow, error)
	SetPageWordBoxes(ctx context.Context, arg contentdb.SetPageWordBoxesParams) error
	SearchPendingBoxPages(ctx context.Context, arg contentdb.SearchPendingBoxPagesParams) ([]int32, error)
	SearchWordBoxes(ctx context.Context, arg contentdb.SearchWordBoxesParams) ([]contentdb.SearchWordBoxesRow, error)

	SearchAllFolders(ctx context.Context, arg contentdb.SearchAllFoldersParams) ([]contentdb.SearchAllFoldersRow, error)
	SearchAllDocuments(ctx context.Context, arg contentdb.SearchAllDocumentsParams) ([]contentdb.SearchAllDocumentsRow, error)
	SearchVisibleFolders(ctx context.Context, arg contentdb.SearchVisibleFoldersParams) ([]contentdb.SearchVisibleFoldersRow, error)
	SearchVisibleDocuments(ctx context.Context, arg contentdb.SearchVisibleDocumentsParams) ([]contentdb.SearchVisibleDocumentsRow, error)
	SearchAllFolderBreadcrumbs(ctx context.Context, folderIds []pgtype.UUID) ([]contentdb.SearchAllFolderBreadcrumbsRow, error)
	SearchVisibleFolderBreadcrumbs(ctx context.Context, arg contentdb.SearchVisibleFolderBreadcrumbsParams) ([]contentdb.SearchVisibleFolderBreadcrumbsRow, error)
	SearchAllContent(ctx context.Context, arg contentdb.SearchAllContentParams) ([]contentdb.SearchAllContentRow, error)
	SearchVisibleContent(ctx context.Context, arg contentdb.SearchVisibleContentParams) ([]contentdb.SearchVisibleContentRow, error)
	SearchAllContentPages(ctx context.Context, arg contentdb.SearchAllContentPagesParams) ([]contentdb.SearchAllContentPagesRow, error)
	SearchVisibleContentPages(ctx context.Context, arg contentdb.SearchVisibleContentPagesParams) ([]contentdb.SearchVisibleContentPagesRow, error)

	CreateArchive(ctx context.Context, arg contentdb.CreateArchiveParams) (contentdb.WorkspaceArchive, error)
	SetArchiveObjectKey(ctx context.Context, arg contentdb.SetArchiveObjectKeyParams) error
	GetArchive(ctx context.Context, arg contentdb.GetArchiveParams) (contentdb.WorkspaceArchive, error)
	ListArchives(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.WorkspaceArchive, error)
	CountPendingArchives(ctx context.Context, workspaceID pgtype.UUID) (int32, error)
	MarkArchiveReady(ctx context.Context, arg contentdb.MarkArchiveReadyParams) error
	MarkArchiveFailed(ctx context.Context, arg contentdb.MarkArchiveFailedParams) error
	DeleteArchive(ctx context.Context, arg contentdb.DeleteArchiveParams) error
	ListExpiredArchives(ctx context.Context) ([]contentdb.ListExpiredArchivesRow, error)
	ListStalePendingArchives(ctx context.Context, cutoff pgtype.Timestamptz) ([]contentdb.ListStalePendingArchivesRow, error)

	CreateDownloadJob(ctx context.Context, arg contentdb.CreateDownloadJobParams) (contentdb.DocumentDownloadJob, error)
	GetDownloadJob(ctx context.Context, id pgtype.UUID) (contentdb.DocumentDownloadJob, error)
	GetPendingDownloadJob(ctx context.Context, arg contentdb.GetPendingDownloadJobParams) (contentdb.DocumentDownloadJob, error)
	ListDownloadJobsForUser(ctx context.Context, arg contentdb.ListDownloadJobsForUserParams) ([]contentdb.DocumentDownloadJob, error)
	MarkDownloadJobReady(ctx context.Context, arg contentdb.MarkDownloadJobReadyParams) error
	MarkDownloadJobFailed(ctx context.Context, arg contentdb.MarkDownloadJobFailedParams) error
	ListStalePendingDownloadJobs(ctx context.Context, cutoff pgtype.Timestamptz) ([]contentdb.DocumentDownloadJob, error)
	ListExpiredDownloadJobs(ctx context.Context) ([]contentdb.DocumentDownloadJob, error)
	DeleteDownloadJob(ctx context.Context, id pgtype.UUID) error
	ListArchiveFolders(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListArchiveFoldersRow, error)
	ListArchiveDocuments(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListArchiveDocumentsRow, error)
	ListArchiveMembers(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListArchiveMembersRow, error)
	ListFolderAccessMatrix(ctx context.Context, workspaceID pgtype.UUID) ([]contentdb.ListFolderAccessMatrixRow, error)
	GetWorkspaceSlugForArchive(ctx context.Context, id pgtype.UUID) (string, error)

	ExecTx(ctx context.Context, fn func(*contentdb.Queries) error) error
	ExecTxTx(ctx context.Context, fn func(*contentdb.Queries, pgx.Tx) error) error
}

// ActivityExporter dan QAExporter dideklarasikan sebagai port, bukan impor
// langsung, karena qa/service sudah bergantung pada ContentService lewat
// ContentAccessChecker — impor balik akan jadi siklus.
type ActivityExporter interface {
	ExportActivityCSV(ctx context.Context, w io.Writer, workspaceID, role string) error
}

type QAExporter interface {
	ExportQuestionsCSV(ctx context.Context, w io.Writer, workspaceID, userID, name, email, role string) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
	RecordPageEvent(ctx context.Context, ev activityservice.PageEvent)
}
