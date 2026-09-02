//go:build !unix

package diskcache

import "math"

func freeBytes(string) (int64, error) {
	return math.MaxInt64, nil
}
