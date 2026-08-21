package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleBBoxXML = `<html><body>
<doc>
  <page width="612.000000" height="792.000000">
    <word xMin="72.000000" yMin="72.000000" xMax="144.000000" yMax="96.000000">Laporan</word>
    <word xMin="150.000000" yMin="72.000000" xMax="300.000000" yMax="96.000000">Keuangan</word>
    <word xMin="72.000000" yMin="120.000000" xMax="180.000000" yMax="144.000000">Tahunan</word>
  </page>
</doc>
</body></html>
`

func TestParseWordBoxes(t *testing.T) {
	words, err := parseWordBoxes([]byte(sampleBBoxXML))
	require.NoError(t, err)

	require.Len(t, words, 3)

	w := words[0]
	assert.Equal(t, "Laporan", w.Text)
	assert.InDelta(t, 72.0/612.0, w.X, 0.0001)
	assert.InDelta(t, 72.0/792.0, w.Y, 0.0001)
	assert.InDelta(t, 72.0/612.0, w.W, 0.0001)
	assert.InDelta(t, 24.0/792.0, w.H, 0.0001)
	assert.Equal(t, 0.0, w.Conf)
}

func TestParseWordBoxesSkipsEmpty(t *testing.T) {
	words, err := parseWordBoxes([]byte(`<html><body><doc><page width="100.000000" height="100.000000"><word xMin="0.000000" yMin="0.000000" xMax="10.000000" yMax="10.000000">   </word></page></doc></body></html>`))
	require.NoError(t, err)
	assert.Empty(t, words)
}

func TestParseWordBoxesMissingPage(t *testing.T) {
	_, err := parseWordBoxes([]byte(`<html><body><doc></doc></body></html>`))
	require.Error(t, err)
}
