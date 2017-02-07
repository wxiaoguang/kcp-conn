package kcp

import "io"

type Buffer struct {
    buf       []byte            // contents are the bytes buf[off : len(buf)]
    off       int               // read at &buf[off], write at &buf[len(buf)]
}

func (b *Buffer) Bytes() []byte { return b.buf[b.off:] }

func (b *Buffer) Len() int { return len(b.buf) - b.off }

func (b *Buffer) Cap() int { return cap(b.buf) }

// Truncate discards all but the first n unread bytes from the buffer
// but continues to use the same allocated storage.
// It panics if n is negative or greater than the length of the buffer.
func (b *Buffer) Truncate(n int) {
    switch {
    case n < 0 || n > b.Len():
        panic("bytes.Buffer: truncation out of range")
    case n == 0:
        // Reuse buffer space.
        b.off = 0
    }
    b.buf = b.buf[0 : b.off+n]
}

// Reset resets the buffer to be empty,
// but it retains the underlying storage for use by future writes.
// Reset is the same as Truncate(0).
func (b *Buffer) Reset() { b.Truncate(0) }

// grow grows the buffer to guarantee space for n more bytes.
// It returns the index where bytes should be written.
// If the buffer can't grow it will panic with ErrTooLarge.
func (b *Buffer) grow(n int) int {
    m := b.Len()
    // If buffer is empty, reset to recover space.
    if m == 0 && b.off != 0 {
        b.Truncate(0)
    }
    if len(b.buf)+n > cap(b.buf) {
        var buf []byte
        if m+n <= cap(b.buf)/2 {
            // We can slide things down instead of allocating a new
            // slice. We only need m+n <= cap(b.buf) to slide, but
            // we instead let capacity get twice as large so we
            // don't spend all our time copying.
            copy(b.buf[:], b.buf[b.off:])
            buf = b.buf[:m]
        } else {
            // not enough space anywhere
            buf = make([]byte, 2*cap(b.buf) + n)
            copy(buf, b.buf[b.off:])
        }
        b.buf = buf
        b.off = 0
    }
    b.buf = b.buf[0 : b.off+m+n]
    return b.off + m
}

// Grow grows the buffer's capacity, if necessary, to guarantee space for
// another n bytes. After Grow(n), at least n bytes can be written to the
// buffer without another allocation.
// If n is negative, Grow will panic.
// If the buffer can't grow it will panic with ErrTooLarge.
func (b *Buffer) Grow(n int) {
    if n < 0 {
        panic("bytes.Buffer.Grow: negative count")
    }
    m := b.grow(n)
    b.buf = b.buf[0:m]
}

// Extend extends the buffer to write n bytes, returns the slice with size n.
// bytes.Buffer lacks this ....
func (b *Buffer) Extend(n int) ([]byte) {
    m := b.grow(n)
    return b.buf[m:]
}


// Write appends the contents of p to the buffer, growing the buffer as
// needed. The return value n is the length of p; err is always nil. If the
// buffer becomes too large, Write will panic with ErrTooLarge.
func (b *Buffer) Write(p []byte) (n int, err error) {
    m := b.grow(len(p))
    return copy(b.buf[m:], p), nil
}


// Next returns a slice containing the next n bytes from the buffer,
// advancing the buffer as if the bytes had been returned by Read.
// If there are fewer than n bytes in the buffer, Next returns the entire buffer.
// The slice is only valid until the next call to a read or write method.
func (b *Buffer) Next(n int) []byte {
    m := b.Len()
    if n > m {
        n = m
    }
    data := b.buf[b.off : b.off+n]
    b.off += n
    return data
}


// Read reads the next len(p) bytes from the buffer or until the buffer
// is drained. The return value n is the number of bytes read. If the
// buffer has no data to return, err is io.EOF (unless len(p) is zero);
// otherwise it is nil.
func (b *Buffer) Read(p []byte) (n int, err error) {
    if b.off >= len(b.buf) {
        // Buffer is empty, reset to recover space.
        b.Truncate(0)
        if len(p) == 0 {
            return
        }
        return 0, io.EOF
    }
    n = copy(p, b.buf[b.off:])
    b.off += n
    return
}

// NewBuffer creates and initializes a new Buffer using buf as its initial
// contents. It is intended to prepare a Buffer to read existing data. It
// can also be used to size the internal buffer for writing. To do that,
// buf should have the desired capacity but a length of zero.
//
// In most cases, new(Buffer) (or just declaring a Buffer variable) is
// sufficient to initialize a Buffer.
func NewBuffer(buf []byte) *Buffer { return &Buffer{buf: buf} }
