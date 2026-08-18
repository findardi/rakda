package convert

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

var (
	ErrUnsupportedFile  = errors.New("unsupported file type")
	ErrConversionFailed = errors.New("conversion failed")
)

type Converter interface {
	ToPDF(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error)
}

var convertible = map[string]struct{}{
	".doc": {}, ".docx": {}, ".odt": {}, ".rtf": {}, ".txt": {},
	".xls": {}, ".xlsx": {}, ".xlsm": {}, ".ods": {}, ".csv": {},
	".ppt": {}, ".pptx": {}, ".odp": {},
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {}, ".svg": {},
}

func Supported(filename string) bool {
	_, ok := convertible[strings.ToLower(filepath.Ext(filename))]
	return ok
}

func IsPDF(filename string) bool {
	return strings.EqualFold(filepath.Ext(filename), ".pdf")
}

func Viewable(filename string) bool {
	return IsPDF(filename) || Supported(filename)
}
