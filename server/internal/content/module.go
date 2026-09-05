package content

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"time"

	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/content/handler"
	"github.com/findardi/rakda/server/internal/content/repository"
	"github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type Module struct {
	handler    *handler.ContentHandler
	mw         *middleware.Middleware
	accessRepo *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, store storage.Storage, viewer service.Viewer, trashRetention time.Duration, activity service.ActivityRecorder, stamp service.StampDeps, archive service.ArchiveDeps, caches service.CacheDeps, rendition service.RenditionDeps) *Module {
	r := repository.New(pool)
	s := service.NewContentService(r, store, viewer, trashRetention, activity, stamp, archive, caches, rendition)
	h := handler.NewContentHandler(s)

	mw := middleware.New(verifier, auth.New(pool), nil)

	return &Module{
		handler:    h,
		mw:         mw,
		accessRepo: accessrepo.New(pool),
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/content", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.accessRepo.ResolveMembership))
			r.Use(m.mw.RequireRoomOpenForGuests)

			r.Group(func(r chi.Router) {
				r.Post("/search/log", m.handler.LogSearch)
				r.Post("/archives", m.handler.CreateArchive)
				r.With(m.mw.RequirePermission(permission.PermDocumentEdit)).
					Post("/documents/{documentID}/versions/{versionID}/retry-rendition", m.handler.RetryRendition)
			})

			r.Group(func(r chi.Router) {
				r.Use(m.mw.RequireRoomWritable)

				r.Get("/search", m.handler.SearchContent)
				r.Get("/search/content/pages", m.handler.SearchContentPages)
				r.Get("/download-limits", m.handler.GetDownloadLimits)

				r.Get("/download-jobs", m.handler.ListDownloadJobs)
				r.With(m.mw.RequirePermission(permission.PermDocumentDownload)).
					Get("/download-jobs/{jobID}", m.handler.GetDownloadJob)
				r.With(m.mw.RequirePermission(permission.PermDocumentDownload)).
					Get("/download-jobs/{jobID}/download", m.handler.DownloadJobArtifact)

				r.Get("/archives", m.handler.ListArchives)
				r.Get("/archives/{archiveID}/download", m.handler.DownloadArchive)
				r.Delete("/archives/{archiveID}", m.handler.DeleteArchive)

				r.Route("/folder-templates", func(r chi.Router) {
					r.With(m.mw.RequirePermission(permission.PermFolderCreate)).Get("/", m.handler.ListFolderTemplates)
					r.With(m.mw.RequirePermission(permission.PermFolderCreate)).Post("/{templateKey}/apply", m.handler.ApplyFolderTemplate)
				})

				r.Route("/folders", func(r chi.Router) {
					r.With(m.mw.RequirePermission(permission.PermFolderView)).Get("/", m.handler.GetFoldersTree)
					r.With(m.mw.RequirePermission(permission.PermFolderCreate)).Post("/", m.handler.CreateFolder)
					r.With(m.mw.RequirePermission(permission.PermFolderCreate)).Post("/bulk", m.handler.BulkCreateFolders)
					r.With(m.mw.RequirePermission(permission.PermFolderDelete)).Post("/bulk-delete", m.handler.BulkDeleteFolders)
					r.With(m.mw.RequirePermission(permission.PermFolderEdit)).Put("/{folderID}", m.handler.RenameFolder)
					r.With(m.mw.RequirePermission(permission.PermFolderEdit)).Patch("/{folderID}/move", m.handler.MoveFolder)
					r.With(m.mw.RequirePermission(permission.PermFolderDelete)).Delete("/{folderID}", m.handler.DeleteFolder)

					r.With(m.mw.RequirePermission(permission.PermGroupView)).Get("/{folderID}/access", m.handler.ListFolderAccess)
					r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Put("/{folderID}/access", m.handler.SetFolderAccess)
					r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Delete("/{folderID}/access/{groupID}", m.handler.RemoveFolderAccess)

					r.With(m.mw.RequirePermission(permission.PermDocumentView)).Get("/{folderID}/documents", m.handler.ListDocuments)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/{folderID}/documents/upload-url", m.handler.RequestUploadURL)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/{folderID}/documents", m.handler.CompletedUpload)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/{folderID}/documents/multipart/init", m.handler.InitMultipart)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/{folderID}/documents/multipart/part-urls", m.handler.MultipartPartURLs)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Get("/{folderID}/documents/multipart/parts", m.handler.MultipartParts)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/{folderID}/documents/multipart/complete", m.handler.CompleteMultipart)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Delete("/{folderID}/documents/multipart", m.handler.AbortMultipart)
				})

				r.With(m.mw.RequirePermission(permission.PermDocumentDelete)).Post("/documents/bulk-delete", m.handler.BulkDeleteDocuments)

				r.Route("/documents/{documentID}", func(r chi.Router) {
					r.With(m.mw.RequirePermission(permission.PermDocumentView)).Get("/versions", m.handler.ListVersions)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/versions/upload-url", m.handler.RequestUploadVersion)
					r.With(m.mw.RequirePermission(permission.PermDocumentUpload)).Post("/versions", m.handler.CompletedVersionUpload)
					r.With(m.mw.RequirePermission(permission.PermDocumentDownload)).Get("/download", m.handler.GetDownloadURL)
					r.With(m.mw.RequirePermission(permission.PermDocumentView)).Get("/view", m.handler.GetViewMeta)
					r.With(m.mw.RequirePermission(permission.PermDocumentView)).Get("/pages/{page}", m.handler.GetViewPage)
					r.With(m.mw.RequirePermission(permission.PermDocumentView)).Get("/search-boxes", m.handler.SearchBoxes)
					r.With(m.mw.RequirePermission(permission.PermDocumentEdit)).Patch("/move", m.handler.MoveDocument)
					r.With(m.mw.RequirePermission(permission.PermDocumentDelete)).Delete("/", m.handler.DeleteDocument)
					r.With(m.mw.RequirePermission(permission.PermDocumentEdit)).Post("/versions/{versionID}/restore", m.handler.RestoreVersion)
				})

				r.Route("/trash", func(r chi.Router) {
					r.Get("/", m.handler.ListTrash)
					r.Post("/folders/{folderID}/restore", m.handler.RestoreTrashFolder)
					r.Post("/documents/{documentID}/restore", m.handler.RestoreTrashDocument)
				})
			})
		})
	})
}
