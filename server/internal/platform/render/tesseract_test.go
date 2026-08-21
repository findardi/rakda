package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleTSV = `1	1	0	0	0	0	0	0	1240	1754	-1	
5	1	1	1	1	1	100	120	200	40	92.5	Laporan
5	1	1	1	1	2	110	160	300	40	88.1	Keuangan
5	1	1	1	2	3	100	200	240	40	74.0	Tahunan
`

func TestParseTesseractTSV(t *testing.T) {
	res, err := parseTesseractTSV([]byte(sampleTSV))
	require.NoError(t, err)

	assert.Equal(t, "Laporan Keuangan\nTahunan", res.Text)
	require.Len(t, res.Words, 3)

	w := res.Words[0]
	assert.Equal(t, "Laporan", w.Text)
	assert.InDelta(t, 100.0/1240.0, w.X, 0.0001)
	assert.InDelta(t, 120.0/1754.0, w.Y, 0.0001)
	assert.InDelta(t, 200.0/1240.0, w.W, 0.0001)
	assert.InDelta(t, 40.0/1754.0, w.H, 0.0001)
	assert.InDelta(t, 92.5, w.Conf, 0.0001)
}

func TestParseTesseractTSVEmpty(t *testing.T) {
	res, err := parseTesseractTSV([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, "", res.Text)
	assert.Empty(t, res.Words)
}

func TestParseTesseractTSVSkipsGarbage(t *testing.T) {
	res, err := parseTesseractTSV([]byte("not a tsv line\n\n5\t1\t1\t1\t1\t1\t0\t0\t0\t0\t90\t"))
	require.NoError(t, err)
	assert.Equal(t, "", res.Text)
	assert.Empty(t, res.Words)
}
