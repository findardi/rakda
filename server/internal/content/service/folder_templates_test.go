package service

import (
	"context"
	"testing"

	"github.com/findardi/rakda/server/internal/content/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func templateDepth(nodes []TemplateNode) int {
	max := 0
	for _, n := range nodes {
		d := 1
		if len(n.Children) > 0 {
			d += templateDepth(n.Children)
		}
		if d > max {
			max = d
		}
	}
	return max
}

func TestFolderTemplatesValid(t *testing.T) {
	require.Len(t, folderTemplates, 5)

	seen := map[string]bool{}
	for _, tpl := range folderTemplates {
		t.Run(tpl.Key, func(t *testing.T) {
			assert.False(t, seen[tpl.Key], "duplicate template key")
			seen[tpl.Key] = true

			assert.NotEmpty(t, tpl.NameID)
			assert.NotEmpty(t, tpl.NameEN)
			assert.NotEmpty(t, tpl.DescID)
			assert.NotEmpty(t, tpl.DescEN)
			assert.LessOrEqual(t, templateDepth(tpl.Folders), 2)

			for _, locale := range []string{"id", "en"} {
				nodes := expandTemplateNodes(tpl.Folders, locale)
				total, err := validateBulkNodes(nodes, 1)
				require.NoError(t, err, "locale %s", locale)
				assert.LessOrEqual(t, total, maxBulkFolderNodes)
				assert.Equal(t, countTemplateNodes(tpl.Folders), total)

				var walk func([]dto.BulkFolderNode)
				walk = func(ns []dto.BulkFolderNode) {
					for _, n := range ns {
						assert.NotEqual(t, "General", n.Name)
						walk(n.Children)
					}
				}
				walk(nodes)
			}
		})
	}
}

func TestExpandTemplateNodesLocale(t *testing.T) {
	nodes := []TemplateNode{
		{NameID: "Keuangan", NameEN: "Financials", Children: []TemplateNode{
			{NameID: "Proyeksi", NameEN: "Projections"},
		}},
	}

	id := expandTemplateNodes(nodes, "id")
	require.Len(t, id, 1)
	assert.Equal(t, "Keuangan", id[0].Name)
	require.Len(t, id[0].Children, 1)
	assert.Equal(t, "Proyeksi", id[0].Children[0].Name)

	en := expandTemplateNodes(nodes, "en")
	assert.Equal(t, "Financials", en[0].Name)
	assert.Equal(t, "Projections", en[0].Children[0].Name)
}

func TestFindFolderTemplate(t *testing.T) {
	tpl, ok := findFolderTemplate("ma-dd")
	require.True(t, ok)
	assert.Equal(t, "ma-dd", tpl.Key)

	_, ok = findFolderTemplate("nonexistent")
	assert.False(t, ok)
}

func TestApplyFolderTemplateUnknownKey(t *testing.T) {
	svc := &ContentService{}
	_, err := svc.ApplyFolderTemplate(context.Background(), dto.ApplyTemplateRequest{
		WorkspaceID: "11111111-1111-1111-1111-111111111111",
		TemplateKey: "nonexistent",
		Locale:      "id",
	}, Actor{UserID: "22222222-2222-2222-2222-222222222222"})
	assert.ErrorIs(t, err, ErrTemplateNotFound)
}

func TestListFolderTemplatesShape(t *testing.T) {
	svc := &ContentService{}
	list := svc.ListFolderTemplates()
	require.Len(t, list, 5)

	maDD, ok := findFolderTemplate("ma-dd")
	require.True(t, ok)
	assert.Equal(t, countTemplateNodes(maDD.Folders), list[0].FolderCount)
	assert.EqualValues(t, 22, list[0].FolderCount)
}
