package kcp

import (
    "net"
    "time"
    "bytes"
    "io"
)

type lossyConn struct {
    conn net.Conn

    chRead chan interface{}
    chReadTricked chan interface{}
    readBuf *bytes.Buffer

    chWrite chan interface{}
    chWriteTricked chan interface{}

    deadlineRead time.Time
    deadlineWrite time.Time
}

func NewLossyConn(conn net.Conn, channelSize int, readTrick, writeTrick LossyTrick) *lossyConn {
    c := &lossyConn{conn: conn}

    c.chRead = make(chan interface{}, channelSize)
    c.chReadTricked = LossyChannel(c.chRead, channelSize, readTrick)
    c.readBuf = &bytes.Buffer{}

    c.chWrite = make(chan interface{}, channelSize)
    c.chWriteTricked = LossyChannel(c.chWrite, channelSize, writeTrick)

    go func() {
        for v := range c.chWriteTricked {
            b := v.([]byte)
            c.conn.Write(b)
        }
    }()

    go func() {
        for {
            c.startRead()
        }
    }()

    return c
}

func (c *lossyConn) startRead() (n int, err error){
    b := make([]byte, 2048)
    n, err = c.conn.Read(b)
    if e, ok := err.(net.Error); ok && e.Timeout() {
        return
    }

    if n == 0 || err != nil {
        close(c.chRead)
        return 0, io.EOF
    }
    c.chRead <- b[:n]
    return n, err
}

func (c *lossyConn) Read(b []byte) (n int, err error) {
    n, _ = c.readBuf.Read(b)
    if n != 0 {
        return n, nil
    }

    var timer *time.Timer
    if !c.deadlineRead.IsZero() {
        delay := c.deadlineRead.Sub(time.Now())
        if delay <= 0 {
            return 0, errTimeout{}
        }
        timer = time.NewTimer(delay)
    } else {
        timer = &time.Timer{}
    }

    select {
    case v := <- c.chReadTricked:
        p := v.([]byte)
        if p == nil {
            return 0, io.EOF
        }

        n, _ = c.readBuf.Write(p)
        n, _ = c.readBuf.Read(b)
    case <-timer.C:
        n = 0
        err = errTimeout{}
    }

    if timer.C != nil {
        timer.Stop()
    }
    return
}

func (c *lossyConn) Write(b []byte) (n int, err error) {
    var timer *time.Timer
    if !c.deadlineWrite.IsZero() {
        delay := c.deadlineWrite.Sub(time.Now())
        if delay <= 0 {
            return 0, errTimeout{}
        }
        timer = time.NewTimer(delay)
    } else {
        timer = &time.Timer{}
    }

    select {
    case c.chWrite <- b:
        n = len(b)
        err = nil
    case <-timer.C:
        n = 0
        err = errTimeout{}
    }

    if timer.C != nil {
        timer.Stop()
    }
    return
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *lossyConn) Close() error {
    return c.conn.Close()
}

// LocalAddr returns the local network address.
func (c *lossyConn) LocalAddr() net.Addr {
    return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *lossyConn) RemoteAddr() net.Addr {
    return c.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail with a timeout (see type Error) instead of
// blocking. The deadline applies to all future I/O, not just
// the immediately following call to Read or Write.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (c *lossyConn) SetDeadline(t time.Time) error {
    c.deadlineRead = t
    c.deadlineWrite = t
    return c.conn.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls.
// A zero value for t means Read will not time out.
func (c *lossyConn) SetReadDeadline(t time.Time) error {
    c.deadlineRead = t
    return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *lossyConn) SetWriteDeadline(t time.Time) error {
    c.deadlineWrite = t
    return c.conn.SetWriteDeadline(t)
}