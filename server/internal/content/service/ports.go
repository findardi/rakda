package service

import (
	"context"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
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

	SearchAllFolders(ctx context.Context, arg contentdb.SearchAllFoldersParams) ([]contentdb.SearchAllFoldersRow, error)
	SearchAllDocuments(ctx context.Context, arg contentdb.SearchAllDocumentsParams) ([]contentdb.SearchAllDocumentsRow, error)
	SearchVisibleFolders(ctx context.Context, arg contentdb.SearchVisibleFoldersParams) ([]contentdb.SearchVisibleFoldersRow, error)
	SearchVisibleDocuments(ctx context.Context, arg contentdb.SearchVisibleDocumentsParams) ([]contentdb.SearchVisibleDocumentsRow, error)
	SearchAllFolderBreadcrumbs(ctx context.Context, folderIds []pgtype.UUID) ([]contentdb.SearchAllFolderBreadcrumbsRow, error)
	SearchVisibleFolderBreadcrumbs(ctx context.Context, arg contentdb.SearchVisibleFolderBreadcrumbsParams) ([]contentdb.SearchVisibleFolderBreadcrumbsRow, error)

	ExecTx(ctx context.Context, fn func(*contentdb.Queries) error) error
	ExecTxTx(ctx context.Context, fn func(*contentdb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
	RecordPageEvent(ctx context.Context, ev activityservice.PageEvent)
}
