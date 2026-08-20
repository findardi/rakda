package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinBreadcrumb(t *testing.T) {
	t.Run("full path when every ancestor is visible", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: true},
			{name: "Mid", visible: true},
			{name: "target", visible: true},
		}
		assert.Equal(t, "Root / Mid / target", joinBreadcrumb(nodes))
	})

	t.Run("cuts at the first invisible ancestor, no ellipsis", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: true},
			{name: "Hidden", visible: false},
			{name: "target", visible: true},
		}
		assert.Equal(t, "Root", joinBreadcrumb(nodes))
	})

	t.Run("empty when the root itself is invisible", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: false},
			{name: "target", visible: true},
		}
		assert.Equal(t, "", joinBreadcrumb(nodes))
	})

	t.Run("empty for no rows", func(t *testing.T) {
		assert.Equal(t, "", joinBreadcrumb(nil))
	})

	t.Run("single visible node", func(t *testing.T) {
		assert.Equal(t, "pitch deck", joinBreadcrumb([]breadcrumbNode{{name: "pitch deck", visible: true}}))
	})
}

func TestBuildSearchQuery(t *testing.T) {
	t.Run("keeps meaningful tokens space-separated", func(t *testing.T) {
		assert.Equal(t, "laporan keuangan", buildSearchQuery("Laporan Keuangan"))
	})

	t.Run("drops Indonesian stopwords", func(t *testing.T) {
		assert.Equal(t, "laporan keuangan", buildSearchQuery("yang dan laporan keuangan"))
	})

	t.Run("drops punctuation and single letters", func(t *testing.T) {
		assert.Equal(t, "pitch deck final", buildSearchQuery("pitch-deck, final!"))
	})

	t.Run("empty when everything is a stopword", func(t *testing.T) {
		assert.Equal(t, "", buildSearchQuery("yang dan di ke"))
	})
}

func TestStripHeadlineMarkup(t *testing.T) {
	assert.Equal(t, "laporan tahunan", stripHeadlineMarkup("laporan <b>tahunan</b>"))
	assert.Equal(t, "a b c", stripHeadlineMarkup("<b>a</b> b c"))
	assert.Equal(t, "no markup", stripHeadlineMarkup("no markup"))
}
