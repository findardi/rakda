package service

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	searchQueryMaxLength = 100
	searchResultLimit    = 20
	contentResultLimit   = 20
	contentPagesLimit    = 10
)

func (s *ContentService) SearchContent(ctx context.Context, workspaceID, query string, actor Actor) (dto.SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return dto.SearchResponse{
			Folders:   []dto.SearchFolderItem{},
			Documents: []dto.SearchDocumentItem{},
			Content:   []dto.SearchContentHit{},
		}, nil
	}
	if len([]rune(query)) > searchQueryMaxLength {
		query = string([]rune(query)[:searchQueryMaxLength])
	}

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return dto.SearchResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	limit := int32(searchResultLimit)

	if actor.bypassesContentAccess() {
		allFolders, err := s.repo.SearchAllFolders(ctx, contentdb.SearchAllFoldersParams{
			WorkspaceID: wID,
			Query:       &query,
			LimitCount:  limit,
		})
		if err != nil {
			return dto.SearchResponse{}, fmt.Errorf("search all folders: %w", err)
		}

		allDocuments, err := s.repo.SearchAllDocuments(ctx, contentdb.SearchAllDocumentsParams{
			WorkspaceID: wID,
			Query:       &query,
			LimitCount:  limit,
		})
		if err != nil {
			return dto.SearchResponse{}, fmt.Errorf("search all documents: %w", err)
		}

		folders := toFolderItems(allFolders)
		documents := toDocumentItems(allDocuments)
		if err := s.attachBreadcrumbs(ctx, wID, folders, documents, actor); err != nil {
			return dto.SearchResponse{}, err
		}

		content, err := s.searchContentL1(ctx, wID, query, actor)
		if err != nil {
			return dto.SearchResponse{}, err
		}

		return dto.SearchResponse{Folders: folders, Documents: documents, Content: content}, nil
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.SearchResponse{}, ErrContentForbidden
	}

	visibleFolders, err := s.repo.SearchVisibleFolders(ctx, contentdb.SearchVisibleFoldersParams{
		WorkspaceID: wID,
		UserID:      uID,
		Query:       &query,
		LimitCount:  limit,
	})
	if err != nil {
		return dto.SearchResponse{}, fmt.Errorf("search visible folders: %w", err)
	}

	visibleDocuments, err := s.repo.SearchVisibleDocuments(ctx, contentdb.SearchVisibleDocumentsParams{
		WorkspaceID: wID,
		UserID:      uID,
		Query:       &query,
		LimitCount:  limit,
	})
	if err != nil {
		return dto.SearchResponse{}, fmt.Errorf("search visible documents: %w", err)
	}

	folders := toFolderItemsVisible(visibleFolders)
	documents := toDocumentItemsVisible(visibleDocuments)
	if err := s.attachBreadcrumbs(ctx, wID, folders, documents, actor); err != nil {
		return dto.SearchResponse{}, err
	}

	content, err := s.searchContentL1(ctx, wID, query, actor)
	if err != nil {
		return dto.SearchResponse{}, err
	}

	return dto.SearchResponse{Folders: folders, Documents: documents, Content: content}, nil
}

// searchContentL1 — hasil isi level 1: satu baris per dokumen, peringkat =
// max skor halaman (bukan sum), hit_count = jumlah halaman yang kena.
func (s *ContentService) searchContentL1(ctx context.Context, wID pgtype.UUID, query string, actor Actor) ([]dto.SearchContentHit, error) {
	searchQuery, _ := buildSearchQuery(query)
	if searchQuery == "" {
		return []dto.SearchContentHit{}, nil
	}

	limit := int32(contentResultLimit)

	var rows []contentHitRow
	if actor.bypassesContentAccess() {
		all, err := s.repo.SearchAllContent(ctx, contentdb.SearchAllContentParams{
			WorkspaceID: wID,
			Query:       searchQuery,
			LimitCount:  limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search all content: %w", err)
		}
		rows = toContentHits(all)
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return nil, ErrContentForbidden
		}

		visible, err := s.repo.SearchVisibleContent(ctx, contentdb.SearchVisibleContentParams{
			WorkspaceID: wID,
			UserID:      uID,
			Query:       searchQuery,
			LimitCount:  limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search visible content: %w", err)
		}
		rows = toContentHitsVisible(visible)
	}

	return s.buildContentHits(ctx, wID, rows, actor)
}

// SearchContentPages — hasil isi level 2: halaman-halaman kena di dalam satu
// dokumen, dengan cuplikan ts_headline memakai konfigurasi yang benar-benar
// cocok (kalau dua-duanya cocok, ikut skor tertinggi). Filter izin tetap di
// dalam query, jadi dokumen yang tidak terlihat pemanggil mengembalikan nol
// halaman, bukan galat.
func (s *ContentService) SearchContentPages(ctx context.Context, workspaceID, documentID, query string, actor Actor) (dto.SearchContentPagesResponse, error) {
	empty := dto.SearchContentPagesResponse{Pages: []dto.SearchContentPage{}}

	query = strings.TrimSpace(query)
	searchQuery, _ := buildSearchQuery(query)
	if searchQuery == "" {
		return empty, nil
	}

	var wID, dID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return empty, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := dID.Scan(documentID); err != nil {
		return empty, ErrDocumentNotFound
	}

	limit := int32(contentPagesLimit)

	if actor.bypassesContentAccess() {
		all, err := s.repo.SearchAllContentPages(ctx, contentdb.SearchAllContentPagesParams{
			DocumentID: dID,
			Query:      searchQuery,
			LimitCount: limit,
		})
		if err != nil {
			return empty, fmt.Errorf("search all content pages: %w", err)
		}

		pages := make([]dto.SearchContentPage, 0, len(all))
		for _, r := range all {
			pages = append(pages, dto.SearchContentPage{PageNo: r.PageNo, Snippet: stripHeadlineMarkup(r.Snippet)})
		}
		return dto.SearchContentPagesResponse{Pages: pages}, nil
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return empty, ErrContentForbidden
	}

	visible, err := s.repo.SearchVisibleContentPages(ctx, contentdb.SearchVisibleContentPagesParams{
		WorkspaceID: wID,
		UserID:      uID,
		DocumentID:  dID,
		Query:       searchQuery,
		LimitCount:  limit,
	})
	if err != nil {
		return empty, fmt.Errorf("search visible content pages: %w", err)
	}

	pages := make([]dto.SearchContentPage, 0, len(visible))
	for _, r := range visible {
		pages = append(pages, dto.SearchContentPage{PageNo: r.PageNo, Snippet: stripHeadlineMarkup(r.Snippet)})
	}
	return dto.SearchContentPagesResponse{Pages: pages}, nil
}

// stripHeadlineMarkup membuang tag <b>/</b> yang ditambahkan ts_headline —
// snippet yang dikirim ke klien teks polos, bukan HTML.
func stripHeadlineMarkup(s string) string {
	s = strings.ReplaceAll(s, "<b>", "")
	s = strings.ReplaceAll(s, "</b>", "")
	return s
}

// LogSearch mencatat kata kunci ke activity_logs ("search", keyword).
// GET /search wajib bebas efek samping, jadi commit-nya lewat endpoint
// terpisah ini; pencarian yang nihil hasil tetap dicatat. targetID mengisi
// kolom nullable target_id — dipakai pencarian di dalam dokumen (9-f).
func (s *ContentService) LogSearch(ctx context.Context, workspaceID, query, targetID string, actor Actor) {
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > searchQueryMaxLength {
		return
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionSearchPerformed, activityservice.TargetSearch,
		targetID, query, nil))
}

// SearchBoxes memulangkan kotak kata yang cocok untuk satu dokumen — larik
// {x,y,w,h} pecahan 0..1, tanpa satu pun string (keputusan 9-f). Halaman
// yang kena secara semantik tapi koordinatnya belum siap masuk ke `pending`,
// bukan dijawab "tidak ditemukan".
func (s *ContentService) SearchBoxes(ctx context.Context, workspaceID, documentID, query string, actor Actor) (dto.SearchBoxesResponse, error) {
	empty := dto.SearchBoxesResponse{Matches: []dto.SearchBoxPage{}, Pending: []int32{}}

	searchQuery, tokens := buildSearchQuery(query)
	if searchQuery == "" {
		return empty, nil
	}

	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return empty, err
	}

	access, err := s.resolveViewAccess(ctx, workspaceID, uuidString(doc.FolderID), actor)
	if err != nil {
		return empty, err
	}
	if !access.canView {
		return empty, ErrContentForbidden
	}

	var dID pgtype.UUID
	if err := dID.Scan(documentID); err != nil {
		return empty, ErrDocumentNotFound
	}

	pendingPages, err := s.repo.SearchPendingBoxPages(ctx, contentdb.SearchPendingBoxPagesParams{
		DocumentID: dID,
		Query:      searchQuery,
	})
	if err != nil {
		return empty, fmt.Errorf("search pending box pages: %w", err)
	}

	boxRows, err := s.repo.SearchWordBoxes(ctx, contentdb.SearchWordBoxesParams{
		DocumentID: dID,
		Query:      searchQuery,
		Tokens:     tokens,
	})
	if err != nil {
		return empty, fmt.Errorf("search word boxes: %w", err)
	}

	boxesByPage := make(map[int32][]dto.SearchBox)
	pageOrder := make([]int32, 0, len(boxRows))
	for _, r := range boxRows {
		if _, seen := boxesByPage[r.PageNo]; !seen {
			pageOrder = append(pageOrder, r.PageNo)
		}
		boxesByPage[r.PageNo] = append(boxesByPage[r.PageNo], dto.SearchBox{
			X: r.X,
			Y: r.Y,
			W: r.W,
			H: r.H,
		})
	}

	matches := make([]dto.SearchBoxPage, 0, len(pageOrder))
	for _, pageNo := range pageOrder {
		matches = append(matches, dto.SearchBoxPage{
			PageNo: pageNo,
			Boxes:  boxesByPage[pageNo],
		})
	}

	pending := make([]int32, 0, len(pendingPages))
	for _, p := range pendingPages {
		if _, has := boxesByPage[p]; has {
			continue
		}
		pending = append(pending, p)
	}

	return dto.SearchBoxesResponse{Matches: matches, Pending: pending}, nil
}

type contentHitRow struct {
	documentID   string
	documentName string
	folderID     string
	pageCount    *int32
	hitCount     int64
}

func toContentHits(rows []contentdb.SearchAllContentRow) []contentHitRow {
	hits := make([]contentHitRow, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, contentHitRow{
			documentID:   uuidString(r.DocumentID),
			documentName: r.DocumentName,
			folderID:     uuidString(r.FolderID),
			pageCount:    r.PageCount,
			hitCount:     r.HitCount,
		})
	}
	return hits
}

func toContentHitsVisible(rows []contentdb.SearchVisibleContentRow) []contentHitRow {
	hits := make([]contentHitRow, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, contentHitRow{
			documentID:   uuidString(r.DocumentID),
			documentName: r.DocumentName,
			folderID:     uuidString(r.FolderID),
			pageCount:    r.PageCount,
			hitCount:     r.HitCount,
		})
	}
	return hits
}

func (s *ContentService) buildContentHits(ctx context.Context, wID pgtype.UUID, rows []contentHitRow, actor Actor) ([]dto.SearchContentHit, error) {
	folderIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		folderIDs = append(folderIDs, r.folderID)
	}

	bc, err := s.breadcrumbMap(ctx, wID, folderIDs, actor)
	if err != nil {
		return nil, err
	}

	hits := make([]dto.SearchContentHit, 0, len(rows))
	for _, r := range rows {
		pageCount := int32(0)
		if r.pageCount != nil {
			pageCount = *r.pageCount
		}

		hits = append(hits, dto.SearchContentHit{
			DocumentID:   r.documentID,
			DocumentName: r.documentName,
			FolderID:     r.folderID,
			Breadcrumb:   bc[r.folderID],
			PageCount:    pageCount,
			HitCount:     r.hitCount,
		})
	}

	return hits, nil
}

// buildSearchQuery menyaring stopword Indonesia dan menyisakan token bersih
// yang diteruskan ke websearch_to_tsquery di SQL. Konfigurasi `indonesian`
// tidak punya daftar stopword (keputusan 9-a), jadi penyaringan ada di sisi
// query; `english` menyaring stopwordnya sendiri di Postgres. `to_tsquery`
// dilarang di sini: ia melempar galat pada input wajar (kurung tak seimbang,
// `&`) — websearch_to_tsquery tidak. Tokens kedua dipakai SearchWordBoxes
// untuk mencocokkan kotak per kata.
func buildSearchQuery(raw string) (string, []string) {
	parts := make([]string, 0, 8)
	for _, word := range strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		w := strings.ToLower(word)
		if _, stop := idStopwords[w]; stop {
			continue
		}
		if len(w) < 2 {
			continue
		}
		parts = append(parts, w)
	}

	return strings.Join(parts, " "), parts
}

// idStopwords — daftar kata fungsi bahasa Indonesia, disaring saat query
// karena konfigurasi FTS `indonesian` tidak menyediakannya.
var idStopwords = map[string]struct{}{
	"yang": {}, "dan": {}, "di": {}, "ke": {}, "dari": {}, "dengan": {}, "untuk": {},
	"pada": {}, "atau": {}, "ini": {}, "itu": {}, "akan": {}, "juga": {}, "oleh": {},
	"saya": {}, "kami": {}, "kita": {}, "anda": {}, "mereka": {}, "adalah": {},
	"ada": {}, "tidak": {}, "bukan": {}, "dalam": {}, "sebagai": {}, "karena": {},
	"jika": {}, "maka": {}, "saat": {}, "setelah": {}, "sebelum": {}, "tentang": {},
	"terhadap": {}, "antara": {}, "melalui": {}, "per": {}, "para": {}, "seperti": {},
	"begitu": {}, "agar": {}, "supaya": {}, "sehingga": {}, "tetapi": {}, "namun": {},
	"walaupun": {}, "meskipun": {}, "tanpa": {}, "sampai": {}, "hingga": {}, "kepada": {},
	"bagi": {}, "olehnya": {}, "selama": {}, "sejak": {}, "kapan": {}, "mana": {},
	"apa": {}, "siapa": {}, "kenapa": {}, "mengapa": {}, "bagaimana": {}, "bila": {},
	"apakah": {}, "yakni": {}, "yaitu": {}, "misalnya": {}, "contoh": {}, "dll": {},
	"dst": {}, "dsb": {}, "hal": {}, "ialah": {}, "atas": {},
	"bawah": {}, "sekitar": {}, "hampir": {}, "selalu": {}, "sering": {}, "kadang": {},
	"pun": {}, "lah": {}, "kah": {}, "kan": {}, "nya": {}, "ku": {}, "mu": {},
}

type breadcrumbNode struct {
	name    string
	visible bool
}

func (s *ContentService) attachBreadcrumbs(ctx context.Context, wID pgtype.UUID, folders []dto.SearchFolderItem, documents []dto.SearchDocumentItem, actor Actor) error {
	ids := make([]string, 0, len(folders)+len(documents))
	for _, f := range folders {
		ids = append(ids, f.ID)
	}
	for _, d := range documents {
		ids = append(ids, d.FolderID)
	}

	bc, err := s.breadcrumbMap(ctx, wID, ids, actor)
	if err != nil {
		return err
	}

	for i := range folders {
		folders[i].Breadcrumb = bc[folders[i].ID]
	}
	for i := range documents {
		documents[i].Breadcrumb = bc[documents[i].FolderID]
	}

	return nil
}

func (s *ContentService) breadcrumbMap(ctx context.Context, wID pgtype.UUID, folderIDs []string, actor Actor) (map[string]string, error) {
	ids := make([]pgtype.UUID, 0, len(folderIDs))
	for _, id := range folderIDs {
		if id == "" {
			continue
		}
		var u pgtype.UUID
		if err := u.Scan(id); err != nil {
			return nil, fmt.Errorf("parse folder id: %w", err)
		}
		ids = append(ids, u)
	}

	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	var rows []breadcrumbRow
	if actor.bypassesContentAccess() {
		all, err := s.repo.SearchAllFolderBreadcrumbs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("breadcrumbs all: %w", err)
		}
		rows = make([]breadcrumbRow, len(all))
		for i, r := range all {
			rows[i] = breadcrumbRow{rootID: uuidString(r.RootID), name: r.Name, visible: r.Visible}
		}
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return nil, ErrContentForbidden
		}

		visible, err := s.repo.SearchVisibleFolderBreadcrumbs(ctx, contentdb.SearchVisibleFolderBreadcrumbsParams{
			WorkspaceID: wID,
			UserID:      uID,
			FolderIds:   ids,
		})
		if err != nil {
			return nil, fmt.Errorf("breadcrumbs visible: %w", err)
		}
		rows = make([]breadcrumbRow, len(visible))
		for i, r := range visible {
			rows[i] = breadcrumbRow{rootID: uuidString(r.RootID), name: r.Name, visible: r.Visible}
		}
	}

	byRoot := make(map[string][]breadcrumbNode)
	for _, r := range rows {
		byRoot[r.rootID] = append(byRoot[r.rootID], breadcrumbNode{name: r.name, visible: r.visible})
	}

	out := make(map[string]string, len(ids))
	for root, nodes := range byRoot {
		out[root] = joinBreadcrumb(nodes)
	}

	return out, nil
}

// joinBreadcrumb merangkai path root→folder dan berhenti sebelum leluhur
// pertama yang tidak terlihat pemanggil, tanpa penanda "…". Leluhur yang
// tidak terlihat bukan sekadar estetika: pohon menyembunyikannya, dan
// menampilkannya di sini membocorkan nama yang sengaja ditutup.
func joinBreadcrumb(nodes []breadcrumbNode) string {
	if len(nodes) == 0 {
		return ""
	}

	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if !n.visible {
			break
		}
		parts = append(parts, n.name)
	}

	return strings.Join(parts, " / ")
}

type breadcrumbRow struct {
	rootID  string
	name    string
	visible bool
}

func toFolderItems(rows []contentdb.SearchAllFoldersRow) []dto.SearchFolderItem {
	items := make([]dto.SearchFolderItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchFolderItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			ParentID: uuidString(r.ParentID),
		})
	}
	return items
}

func toFolderItemsVisible(rows []contentdb.SearchVisibleFoldersRow) []dto.SearchFolderItem {
	items := make([]dto.SearchFolderItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchFolderItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			ParentID: uuidString(r.ParentID),
		})
	}
	return items
}

func toDocumentItems(rows []contentdb.SearchAllDocumentsRow) []dto.SearchDocumentItem {
	items := make([]dto.SearchDocumentItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchDocumentItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			FolderID: uuidString(r.FolderID),
			Mime:     deref(r.Mime),
		})
	}
	return items
}

func toDocumentItemsVisible(rows []contentdb.SearchVisibleDocumentsRow) []dto.SearchDocumentItem {
	items := make([]dto.SearchDocumentItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchDocumentItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			FolderID: uuidString(r.FolderID),
			Mime:     deref(r.Mime),
		})
	}
	return items
}
