package kcp

import (
    "time"
    "net"
)

type BufferConn struct {
    buf *Buffer
    conn net.Conn
}

func NewBufferConn(c net.Conn) *BufferConn {
    return &BufferConn{
        buf: &Buffer{},
        conn: c,
    }
}

func (c *BufferConn) BufferBytes() []byte { return c.buf.Bytes() }

func (c *BufferConn) BufferLen() int { return c.buf.Len() }

func (c *BufferConn) ReadToBuffer(sz int) (int, error) {
    return c.conn.Read(c.buf.Extend(sz))
}

func (c *BufferConn) Read(b []byte) (int, error) {
    if c.buf.Len() > 0 {
        return c.buf.Read(b)
    }
    return c.buf.Read(b)
}

func (c *BufferConn) Write(b []byte) (int, error) {
    return c.conn.Write(b)
}

func (c *BufferConn) Close() error {
    return c.conn.Close()
}

func (c *BufferConn) LocalAddr() net.Addr {
    return c.conn.LocalAddr()
}

func (c *BufferConn) RemoteAddr() net.Addr {
    return c.conn.RemoteAddr()
}

func (c *BufferConn) SetDeadline(t time.Time) error {
    return c.conn.SetDeadline(t)
}

func (c *BufferConn) SetReadDeadline(t time.Time) error {
    return c.conn.SetReadDeadline(t)
}

func (c *BufferConn) SetWriteDeadline(t time.Time) error {
    return c.conn.SetWriteDeadline(t)
}
