package diskcache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	magic        = "RKDC"
	formatV1     = 1
	saltSize     = 16
	tagSize      = 16
	nonceSize    = 12
	chunkSize    = 1 << 20
	maxChunkSize = 64 << 20
	fixedHdrSize = len(magic) + 1 + saltSize + 2
	hkdfInfo     = "rakda-diskcache"
	MasterKeyLen = 32
)

var (
	ErrBadHeader = errors.New("diskcache: bad header")
	ErrCorrupt   = errors.New("diskcache: corrupt entry")
)

type header struct {
	salt      [saltSize]byte
	key       string
	chunkSize uint32
	raw       []byte
}

func newHeader(key string) (header, error) {
	h := header{key: key, chunkSize: chunkSize}
	if _, err := rand.Read(h.salt[:]); err != nil {
		return header{}, fmt.Errorf("salt: %w", err)
	}

	h.raw = h.encode()
	return h, nil
}

func (h header) encode() []byte {
	buf := make([]byte, 0, fixedHdrSize+len(h.key)+4)
	buf = append(buf, magic...)
	buf = append(buf, formatV1)
	buf = append(buf, h.salt[:]...)
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(h.key)))
	buf = append(buf, h.key...)
	buf = binary.BigEndian.AppendUint32(buf, h.chunkSize)
	return buf
}

func readHeader(r io.Reader) (header, error) {
	fixed := make([]byte, fixedHdrSize)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return header{}, fmt.Errorf("%w: %v", ErrBadHeader, err)
	}

	if string(fixed[:len(magic)]) != magic || fixed[len(magic)] != formatV1 {
		return header{}, ErrBadHeader
	}

	var h header
	copy(h.salt[:], fixed[len(magic)+1:])

	keyLen := int(binary.BigEndian.Uint16(fixed[len(magic)+1+saltSize:]))
	if keyLen == 0 {
		return header{}, ErrBadHeader
	}

	rest := make([]byte, keyLen+4)
	if _, err := io.ReadFull(r, rest); err != nil {
		return header{}, fmt.Errorf("%w: %v", ErrBadHeader, err)
	}

	h.key = string(rest[:keyLen])
	h.chunkSize = binary.BigEndian.Uint32(rest[keyLen:])
	if h.chunkSize == 0 || h.chunkSize > maxChunkSize {
		return header{}, ErrBadHeader
	}

	h.raw = append(fixed, rest...)
	return h, nil
}

func newAEAD(masterKey, salt []byte) (cipher.AEAD, error) {
	fileKey, err := hkdf.Key(sha256.New, masterKey, salt, hkdfInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(fileKey)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

func chunkNonce(counter uint64, last bool) []byte {
	nonce := make([]byte, nonceSize)
	binary.BigEndian.PutUint64(nonce[3:11], counter)
	if last {
		nonce[11] = 1
	}

	return nonce
}

type encWriter struct {
	w       io.Writer
	aead    cipher.AEAD
	aad     []byte
	buf     []byte
	n       int
	counter uint64
	out     []byte
	err     error
}

func newEncWriter(w io.Writer, aead cipher.AEAD, aad []byte, chunk int) *encWriter {
	return &encWriter{
		w:    w,
		aead: aead,
		aad:  aad,
		buf:  make([]byte, chunk),
		out:  make([]byte, 0, chunk+tagSize),
	}
}

func (e *encWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}

	written := 0
	for len(p) > 0 {
		if e.n == len(e.buf) {
			if err := e.flush(false); err != nil {
				return written, err
			}
		}

		n := copy(e.buf[e.n:], p)
		e.n += n
		p = p[n:]
		written += n
	}

	return written, nil
}

func (e *encWriter) flush(last bool) error {
	e.out = e.aead.Seal(e.out[:0], chunkNonce(e.counter, last), e.buf[:e.n], e.aad)
	if _, err := e.w.Write(e.out); err != nil {
		e.err = err
		return err
	}

	e.counter++
	e.n = 0
	return nil
}

func (e *encWriter) Close() error {
	if e.err != nil {
		return e.err
	}

	return e.flush(true)
}

type Reader struct {
	f         *os.File
	key       string
	aead      cipher.AEAD
	aad       []byte
	hdrLen    int64
	chunk     int64
	nChunks   int64
	size      int64
	pos       int64
	plain     []byte
	plainIdx  int64
	cipherBuf []byte
	onCorrupt func()
	failed    error
}

func openReader(path string, masterKey []byte, onCorrupt func()) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	r, err := newReader(f, masterKey, onCorrupt)
	if err != nil {
		f.Close()
		return nil, err
	}

	return r, nil
}

func newReader(f *os.File, masterKey []byte, onCorrupt func()) (*Reader, error) {
	h, err := readHeader(f)
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	hdrLen := int64(len(h.raw))
	chunk := int64(h.chunkSize)
	body := fi.Size() - hdrLen
	if body < tagSize {
		return nil, ErrCorrupt
	}

	frame := chunk + tagSize
	nChunks := (body + frame - 1) / frame
	if lastLen := body - (nChunks-1)*frame; lastLen < tagSize {
		return nil, ErrCorrupt
	}

	aead, err := newAEAD(masterKey, h.salt[:])
	if err != nil {
		return nil, err
	}

	return &Reader{
		f:         f,
		key:       h.key,
		aead:      aead,
		aad:       h.raw,
		hdrLen:    hdrLen,
		chunk:     chunk,
		nChunks:   nChunks,
		size:      body - nChunks*tagSize,
		plainIdx:  -1,
		onCorrupt: onCorrupt,
	}, nil
}

func (r *Reader) Key() string { return r.key }

func (r *Reader) Size() int64 { return r.size }

func (r *Reader) Read(p []byte) (int, error) {
	if r.failed != nil {
		return 0, r.failed
	}

	if r.pos >= r.size {
		return 0, io.EOF
	}

	idx := r.pos / r.chunk
	if idx != r.plainIdx {
		if err := r.load(idx); err != nil {
			return 0, err
		}
	}

	off := r.pos - idx*r.chunk
	n := copy(p, r.plain[off:])
	r.pos += int64(n)
	return n, nil
}

func (r *Reader) load(idx int64) error {
	frame := r.chunk + tagSize
	last := idx == r.nChunks-1

	clen := frame
	if last {
		clen = r.size - idx*r.chunk + tagSize
	}

	if int64(cap(r.cipherBuf)) < clen {
		r.cipherBuf = make([]byte, clen)
	}
	buf := r.cipherBuf[:clen]

	if _, err := r.f.ReadAt(buf, r.hdrLen+idx*frame); err != nil {
		return r.corrupt(err)
	}

	plain, err := r.aead.Open(r.plain[:0], chunkNonce(uint64(idx), last), buf, r.aad)
	if err != nil {
		return r.corrupt(err)
	}

	r.plain = plain
	r.plainIdx = idx
	return nil
}

func (r *Reader) corrupt(cause error) error {
	r.failed = fmt.Errorf("%w: %v", ErrCorrupt, cause)
	if r.onCorrupt != nil {
		r.onCorrupt()
		r.onCorrupt = nil
	}

	return r.failed
}

func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, errors.New("diskcache: invalid whence")
	}

	if abs < 0 {
		return 0, errors.New("diskcache: negative position")
	}

	r.pos = abs
	return abs, nil
}

func (r *Reader) Close() error {
	return r.f.Close()
}
