package kcp

import (
    "testing"
    "net"
    "time"
    "fmt"
    "strconv"
    "strings"
    "math/rand"
    "errors"
    "sync/atomic"
    "log"
)

type testConnTrick struct {
    delayMs int
    lossRatio float64
    limitPerSecond int
}

func (t *testConnTrick) DelayMs() int {
    return t.delayMs
}

func (t *testConnTrick) LossRatio() float64 {
    return t.lossRatio
}

func (t *testConnTrick) LimitPerSecond() int {
    return t.limitPerSecond
}

type testMockLossyConn struct {
    countFeed  int
    countWrite int
    chFeed     chan []byte
}

func testLossyConnNowMs() int {
    return int(time.Now().UnixNano() / int64(time.Millisecond))
}

func (c *testMockLossyConn) init() {
    //make the channel large enough
    c.chFeed = make(chan []byte, 100000)
}

func (c *testMockLossyConn) feed() bool {
    s := fmt.Sprintf("%d\n", testLossyConnNowMs())
    b := make([]byte, 100 + rand.Intn(1000))
    copy(b, []byte(s))
    select {
    case c.chFeed <- b:
        c.countFeed++
        return true
    default:
        return false
    }
}

func (c *testMockLossyConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
    select {
    case tmp := <- c.chFeed:
        if len(b) < len(tmp) {
            panic("not enough buffer")
        }
        copy(b, tmp)
        return len(tmp), nil, nil
    case <- time.After(500 * time.Millisecond):
        return 0, nil, errTimeout{}
    }
    return 0, nil, errors.New(errBrokenPipe)
}

func (c *testMockLossyConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
    c.countWrite++
    s := string(b)
    now := testLossyConnNowMs()
    t, _ := strconv.Atoi(s)
    fmt.Printf("write: %d\n", now - t)
    return len(b), nil
}

func (c *testMockLossyConn) Close() error {
    return nil
}
func (c *testMockLossyConn) LocalAddr() net.Addr {
    return nil
}
func (c *testMockLossyConn) SetDeadline(t time.Time) error {
    return nil
}
func (c *testMockLossyConn) SetReadDeadline(t time.Time) error {
    return nil
}
func (c *testMockLossyConn) SetWriteDeadline(t time.Time) error {
    return nil
}

func TestLossyConnReadWrite(test *testing.T) {
    readTrick := &testConnTrick{delayMs: 20, lossRatio: 0.01}
    writeTrick := &testConnTrick{delayMs: 50, lossRatio: 0.05}

    c := &testMockLossyConn{}
    c.init()
    nc := NewLossyConn(c, 1024, readTrick, writeTrick)

    count := 500
    countReadLoss := 0
    for j := 0; j < readTrick.delayMs; j++ {
        if c.countFeed < count {
            c.feed()
        }
        time.Sleep(1 * time.Millisecond)
    }

    for i := 0; i < count; i++ {
        if c.countFeed < count {
            c.feed()
        }

        buf := make([]byte, 4096)
        nc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, _, err := nc.ReadFrom(buf)
        if err != nil {
            fmt.Printf("read err=%v\n", err)
            countReadLoss++
        } else {
            now := testLossyConnNowMs()
            buf = buf[:n]
            s := string(buf)
            a := strings.Split(s, "\n")
            t, _ := strconv.Atoi(a[0])
            fmt.Printf("read: %d (%s)\n", now - t, a[0])
        }

        now := testLossyConnNowMs()
        nc.WriteTo([]byte(strconv.Itoa(now)), nil)
    }
    nc.Close()

    <- time.After(1 * time.Second)
    fmt.Printf("done, count=%d, countFeed=%d, countReadLoss=%d, countWrite=%d\n", count, c.countFeed, countReadLoss, c.countWrite)
}


func TestLossyConnSpeed(test *testing.T) {
    readTrick := &testConnTrick{delayMs: 20, lossRatio: 0.01, limitPerSecond: 50 * 1024}
    writeTrick := &testConnTrick{delayMs: 50, lossRatio: 0.05}

    c := &testMockLossyConn{}
    c.init()

    nc := NewLossyConn(c, 1024, readTrick, writeTrick)

    go func() {
        for {
            c.feed()
        }
    }()

    totalSize := int64(0)
    go func() {
        buf := make([]byte, 4096)
        for {
            n, _, _ := nc.ReadFrom(buf)
            atomic.AddInt64(&totalSize, int64(n))
        }
    }()

    for i := 0; i < 3; i++ {
        <- time.After(1 * time.Second)
        sz := atomic.SwapInt64(&totalSize, 0)
        fmt.Printf("speed=%.2f KB/s\n", float64(sz) / 1024.0)
    }

    nc.Close()
    <- time.After(1 * time.Second)

}



func TestLossyPairConnSpeed(test *testing.T) {
    trick1 := &testConnTrick{delayMs: 200, lossRatio: 0.02}
    trick2 := &testConnTrick{delayMs: 200, lossRatio: 0.02}

    lc1, lc2 := NewLossyPairConn(1024, trick1, trick2)

    closed := false
    go func() {
        b := make([]byte, 4096)
        for !closed {
            lc1.WriteTo(b, nil)
        }
    }()

    recvSize := int64(0)
    go func() {
        b := make([]byte, 4096)
        for !closed {
            n, _, _ := lc2.ReadFrom(b)
            atomic.AddInt64(&recvSize, int64(n))
        }
    }()

    ch := make(chan int, 1)
    go func() {
        for i := 0; i < 3; i++ {
            start := time.Now()
            time.Sleep(time.Second)
            sz := atomic.SwapInt64(&recvSize, 0)
            mb := (float64(sz)/1024.0/1024.0)
            d := time.Now().Sub(start)
            log.Printf("time for %.2f MB : %v, speed=%.2f MB/s\n", mb, d, mb / d.Seconds())
        }
        close(ch)
    }()

    <- ch
    closed = true

}
