package service

import (
	"context"
	"errors"
	"fmt"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrTemplateNotFound = errors.New("template not found")

type TemplateNode struct {
	NameID   string
	NameEN   string
	Children []TemplateNode
}

type FolderTemplate struct {
	Key    string
	NameID string
	NameEN string
	DescID string
	DescEN string

	Folders []TemplateNode
}

var folderTemplates = []FolderTemplate{
	{
		Key:    "ma-dd",
		NameID: "Due diligence M&A",
		NameEN: "M&A due diligence",
		DescID: "Jual-beli perusahaan — checklist lengkap untuk pemeriksaan seluruh aspek bisnis.",
		DescEN: "Buying or selling a company — a full checklist covering every aspect of the business.",
		Folders: []TemplateNode{
			{NameID: "Ringkasan perusahaan", NameEN: "Company overview"},
			{NameID: "Keuangan", NameEN: "Financials", Children: []TemplateNode{
				{NameID: "Laporan historis", NameEN: "Historical statements"},
				{NameID: "Laporan manajemen", NameEN: "Management accounts"},
				{NameID: "Proyeksi", NameEN: "Projections"},
			}},
			{NameID: "Legal & korporasi", NameEN: "Legal & corporate", Children: []TemplateNode{
				{NameID: "Dokumen korporasi", NameEN: "Corporate documents"},
				{NameID: "Perjanjian pemegang saham", NameEN: "Shareholder agreements"},
				{NameID: "Litigasi & sengketa", NameEN: "Litigation & disputes"},
				{NameID: "Perizinan & regulasi", NameEN: "Licenses & regulatory"},
			}},
			{NameID: "Pajak", NameEN: "Tax"},
			{NameID: "Kontrak & komersial", NameEN: "Contracts & commercial", Children: []TemplateNode{
				{NameID: "Kontrak pelanggan", NameEN: "Customer contracts"},
				{NameID: "Kontrak pemasok", NameEN: "Supplier contracts"},
				{NameID: "Kemitraan & NDA", NameEN: "Partnerships & NDAs"},
			}},
			{NameID: "SDM", NameEN: "Human resources"},
			{NameID: "Kekayaan intelektual", NameEN: "Intellectual property"},
			{NameID: "Produk & teknologi", NameEN: "Product & technology"},
			{NameID: "Pelanggan & penjualan", NameEN: "Customers & sales"},
			{NameID: "Aset & asuransi", NameEN: "Assets & insurance"},
			{NameID: "Kepatuhan & risiko", NameEN: "Compliance & risk"},
			{NameID: "Dokumen closing", NameEN: "Closing documents"},
		},
	},
	{
		Key:    "fundraising",
		NameID: "Fundraising",
		NameEN: "Fundraising",
		DescID: "Penggalangan dana — ruangan ringkas yang bercerita ke investor, dari pitch sampai traksi.",
		DescEN: "Raising capital — a compact room that tells investors the story, from pitch to traction.",
		Folders: []TemplateNode{
			{NameID: "Ringkasan & pitch", NameEN: "Overview & pitch"},
			{NameID: "Keuangan", NameEN: "Financials", Children: []TemplateNode{
				{NameID: "Laporan keuangan", NameEN: "Financial statements"},
				{NameID: "Proyeksi & model", NameEN: "Projections & model"},
			}},
			{NameID: "Legal & struktur", NameEN: "Legal & structure", Children: []TemplateNode{
				{NameID: "Dokumen korporasi", NameEN: "Corporate documents"},
				{NameID: "Cap table & riwayat pendanaan", NameEN: "Cap table & funding history"},
			}},
			{NameID: "Produk & teknologi", NameEN: "Product & technology"},
			{NameID: "Traksi & metrik", NameEN: "Traction & metrics"},
			{NameID: "Pasar & kompetisi", NameEN: "Market & competition"},
			{NameID: "Tim", NameEN: "Team"},
		},
	},
	{
		Key:    "property",
		NameID: "Transaksi properti",
		NameEN: "Property transaction",
		DescID: "Jual-beli atau pemeriksaan satu aset properti — berpusat pada objek, bukan perusahaan.",
		DescEN: "Sale or review of a single property asset — centred on the object, not a company.",
		Folders: []TemplateNode{
			{NameID: "Kepemilikan & sertifikat", NameEN: "Title & ownership"},
			{NameID: "Sewa & penghuni", NameEN: "Leases & tenancy"},
			{NameID: "Keuangan properti", NameEN: "Property financials"},
			{NameID: "Perizinan & tata ruang", NameEN: "Permits & zoning"},
			{NameID: "Inspeksi & lingkungan", NameEN: "Inspections & environmental"},
			{NameID: "Kontrak pengelolaan", NameEN: "Management contracts"},
			{NameID: "Asuransi & sengketa", NameEN: "Insurance & disputes"},
		},
	},
	{
		Key:    "audit",
		NameID: "Audit & pelaporan",
		NameEN: "Audit & reporting",
		DescID: "Audit atau pelaporan berkala — bukti per periode buku untuk auditor.",
		DescEN: "An audit or recurring reporting cycle — evidence per book period for the auditors.",
		Folders: []TemplateNode{
			{NameID: "Laporan keuangan", NameEN: "Financial statements"},
			{NameID: "Buku besar & rekonsiliasi", NameEN: "General ledger & reconciliations"},
			{NameID: "Pajak", NameEN: "Tax"},
			{NameID: "Perbankan & kas", NameEN: "Banking & cash"},
			{NameID: "Kontrak material", NameEN: "Material contracts"},
			{NameID: "Kebijakan & SOP", NameEN: "Policies & SOPs"},
			{NameID: "Korespondensi auditor", NameEN: "Auditor correspondence"},
		},
	},
	{
		Key:    "legal",
		NameID: "Legal & litigasi",
		NameEN: "Legal & litigation",
		DescID: "Perkara atau sengketa — kronologi dan bukti untuk kuasa hukum.",
		DescEN: "A case or dispute — chronology and evidence for counsel.",
		Folders: []TemplateNode{
			{NameID: "Dokumen inti perkara", NameEN: "Core case documents"},
			{NameID: "Korespondensi", NameEN: "Correspondence"},
			{NameID: "Bukti & catatan internal", NameEN: "Evidence & internal records"},
			{NameID: "Keuangan terkait", NameEN: "Related financials"},
			{NameID: "Laporan ahli", NameEN: "Expert reports"},
			{NameID: "Pengajuan & putusan", NameEN: "Filings & rulings"},
		},
	},
}

func findFolderTemplate(key string) (FolderTemplate, bool) {
	for _, tpl := range folderTemplates {
		if tpl.Key == key {
			return tpl, true
		}
	}
	return FolderTemplate{}, false
}

func expandTemplateNodes(nodes []TemplateNode, locale string) []dto.BulkFolderNode {
	out := make([]dto.BulkFolderNode, 0, len(nodes))
	for _, n := range nodes {
		name := n.NameID
		if locale == "en" {
			name = n.NameEN
		}
		out = append(out, dto.BulkFolderNode{
			Name:     name,
			Children: expandTemplateNodes(n.Children, locale),
		})
	}
	return out
}

func countTemplateNodes(nodes []TemplateNode) int {
	total := 0
	for _, n := range nodes {
		total += 1 + countTemplateNodes(n.Children)
	}
	return total
}

func templateNodeResponses(nodes []TemplateNode) []dto.TemplateNodeResponse {
	out := make([]dto.TemplateNodeResponse, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, dto.TemplateNodeResponse{
			NameID:   n.NameID,
			NameEN:   n.NameEN,
			Children: templateNodeResponses(n.Children),
		})
	}
	return out
}

func (s *ContentService) ListFolderTemplates() []dto.FolderTemplateResponse {
	out := make([]dto.FolderTemplateResponse, 0, len(folderTemplates))
	for _, tpl := range folderTemplates {
		out = append(out, dto.FolderTemplateResponse{
			Key:         tpl.Key,
			NameID:      tpl.NameID,
			NameEN:      tpl.NameEN,
			DescID:      tpl.DescID,
			DescEN:      tpl.DescEN,
			FolderCount: countTemplateNodes(tpl.Folders),
			Folders:     templateNodeResponses(tpl.Folders),
		})
	}
	return out
}

func (s *ContentService) ApplyFolderTemplate(ctx context.Context, req dto.ApplyTemplateRequest, actor Actor) (dto.ApplyTemplateResponse, error) {
	tpl, ok := findFolderTemplate(req.TemplateKey)
	if !ok {
		return dto.ApplyTemplateResponse{}, ErrTemplateNotFound
	}

	var wID, cID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.ApplyTemplateResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}
	if err := cID.Scan(actor.UserID); err != nil {
		return dto.ApplyTemplateResponse{}, fmt.Errorf("user id parse: %w", err)
	}

	nodes := expandTemplateNodes(tpl.Folders, req.Locale)
	total, err := validateBulkNodes(nodes, 1)
	if err != nil {
		return dto.ApplyTemplateResponse{}, err
	}
	if total > maxBulkFolderNodes {
		return dto.ApplyTemplateResponse{}, ErrBulkTooManyFolders
	}

	name := tpl.NameID
	if req.Locale == "en" {
		name = tpl.NameEN
	}

	var createdCount, skippedCount int
	out, err := s.runFolderTreeTx(ctx, wID, pgtype.UUID{}, cID, nodes, func(tx pgx.Tx, out []dto.BulkFolderResult, created int) error {
		createdCount = created
		skippedCount = len(out) - created

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionTemplateApplied, activityservice.TargetFolder,
			"", name, map[string]any{"template": tpl.Key, "created": created, "skipped": skippedCount}))
	})
	if err != nil {
		return dto.ApplyTemplateResponse{}, err
	}

	return dto.ApplyTemplateResponse{
		Folders:      out,
		CreatedCount: createdCount,
		SkippedCount: skippedCount,
		Template:     tpl.Key,
	}, nil
}
