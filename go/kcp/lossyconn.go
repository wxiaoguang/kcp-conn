package kcp

import (
    "net"
    "time"
    "io"
)

type lossyPacketConn struct {
    conn net.PacketConn

    chRead chan interface{}
    chReadTricked chan interface{}

    chWrite chan interface{}
    chWriteTricked chan interface{}

    deadlineRead time.Time
    deadlineWrite time.Time

    die chan struct{}
}

type lossyPacket struct {
    data []byte
    addr net.Addr
}

func NewLossyConn(conn net.PacketConn, channelSize int, readTrick, writeTrick LossyTrick) *lossyPacketConn {
    c := &lossyPacketConn{conn: conn}

    c.die = make(chan struct{}, 1)
    c.chRead = make(chan interface{}, channelSize)
    c.chReadTricked = LossyChannel("read", c.chRead, channelSize, readTrick)

    c.chWrite = make(chan interface{}, channelSize)
    c.chWriteTricked = LossyChannel("write", c.chWrite, channelSize, writeTrick)

    go func() {
        for v := range c.chWriteTricked {
            p := v.(*lossyPacket)
            c.conn.WriteTo(p.data, p.addr)
        }
    }()

    go func() {
        loop:
        for c.chWrite != nil {
            b := make([]byte, 4 * 1024)
            c.conn.SetReadDeadline(time.Now().Add(900 * time.Millisecond))
            n, addr, err := c.conn.ReadFrom(b)
            if e, ok := err.(net.Error); ok && e.Timeout() {
                continue
            }

            if n == 0 || err != nil{
                break loop
            }

            p := &lossyPacket{data: b, addr: addr}
            select {
            case c.chRead <- p:
            case <- c.die:
                break loop
            }
        }
        close(c.chRead)
    }()

    return c
}

func (c *lossyPacketConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
    var timer *time.Timer
    if !c.deadlineRead.IsZero() {
        delay := c.deadlineRead.Sub(time.Now())
        if delay <= 0 {
            return 0, nil, errTimeout{}
        }
        timer = time.NewTimer(delay)
    } else {
        timer = &time.Timer{}
    }

    select {
    case v := <- c.chReadTricked:
        if v == nil {
            return 0, nil, io.EOF
        }

        p := v.(*lossyPacket)
        if len(b) < len(p.data) {
            panic("buffer too small")
        }
        copy(b, p.data)
        return len(p.data), p.addr, nil

    case <-timer.C:
        n = 0
        err = errTimeout{}
    }

    if timer.C != nil {
        timer.Stop()
    }
    return
}

func (c *lossyPacketConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
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

    p := &lossyPacket{}
    p.data = make([]byte, len(b))
    copy(p.data, b)
    p.addr = addr
    select {
    case c.chWrite <- p:
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

func (c *lossyPacketConn) Close() error {
    close(c.die)

    if c.chWrite != nil {
        close(c.chWrite)
        c.chWrite = nil
    }

    return c.conn.Close()
}

func (c *lossyPacketConn) LocalAddr() net.Addr {
    return c.conn.LocalAddr()
}

func (c *lossyPacketConn) SetDeadline(t time.Time) error {
    c.deadlineRead = t
    c.deadlineWrite = t
    return c.conn.SetDeadline(t)
}

func (c *lossyPacketConn) SetReadDeadline(t time.Time) error {
    c.deadlineRead = t
    return c.conn.SetReadDeadline(t)
}

func (c *lossyPacketConn) SetWriteDeadline(t time.Time) error {
    c.deadlineWrite = t
    return c.conn.SetWriteDeadline(t)
}




type lossyPairConn struct {
    chRead chan interface{}
    chWrite chan interface{}
}

func NewLossyPairConn(sz int, lossyTrick1, lossyTrick2 LossyTrick) (net.PacketConn, net.PacketConn) {
    ch12 := make(chan interface{}, sz)
    ch21 := make(chan interface{}, sz)

    c1 := &lossyPairConn{
        chRead: LossyChannel("1.read", ch21, sz, lossyTrick2),
        chWrite: ch12,
    }

    c2 := &lossyPairConn{
        chRead: LossyChannel("2.read", ch12, sz, lossyTrick1),
        chWrite: ch21,
    }
    return c1, c2
}

func (c *lossyPairConn) ReadFrom(b []byte) (int, net.Addr, error) {
    v := <- c.chRead
    if v == nil {
        return 0, nil, io.EOF
    }

    p := v.([]byte)
    if len(b) < len(p) {
        panic("buffer too small")
    }
    copy(b, p)
    //log.Printf("ReadFrom %d %+v\n", len(p), p[:24])
    return len(p), nil, nil
}

func (c *lossyPairConn) WriteTo(b []byte, addr net.Addr) (int, error) {
    //log.Printf("WriteTo %d %+v\n", len(b), b[:24])
    p := make([]byte, len(b))
    copy(p, b)
    select {
        case c.chWrite <- p:
            return len(p), nil
        default:
            //print("lossyPairConn write lost")

    }
    return 0, io.ErrShortWrite
}

func (c *lossyPairConn) Close() error {
    close(c.chWrite)
    return nil
}

func (c *lossyPairConn) LocalAddr() net.Addr {
    return nil
}

func (c *lossyPairConn) SetDeadline(t time.Time) error {
    return nil
}

func (c *lossyPairConn) SetReadDeadline(t time.Time) error {
    return nil
}

func (c *lossyPairConn) SetWriteDeadline(t time.Time) error {
    return nil
}