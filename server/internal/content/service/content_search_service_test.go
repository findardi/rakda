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
