package kcp

import (
    "testing"
    "net"
    "time"
    "fmt"
    "strconv"
    "strings"
)

type testConnTrick struct {
    delayMs int
    lossRatio float64
}

func (t *testConnTrick) DelayMs() int {
    return t.delayMs
}

func (t *testConnTrick) LossRatio() float64 {
    return t.lossRatio
}

type testMockLossyConn struct {
    countFeed  int
    countWrite int
    chFeed     chan []byte
}

func testLossyConnNowMs() int {
    return int(time.Now().UnixNano() / int64(time.Millisecond))
}

func (c *testMockLossyConn) feed() {
    if c.chFeed == nil {
        c.chFeed = make(chan []byte, 1024)
    }
    s := fmt.Sprintf("%d\n", testLossyConnNowMs())
    c.countFeed++
    c.chFeed <- []byte(s)
}
func (c *testMockLossyConn) Read(b []byte) (n int, err error) {
    tmp := <- c.chFeed
    copy(b, tmp)
    return len(tmp), nil
}

func (c *testMockLossyConn) Write(b []byte) (n int, err error) {
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
func (c *testMockLossyConn) RemoteAddr() net.Addr {
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

func TestLossyConn(test *testing.T) {
    //readTrick := &testConnTrick{delayMs: 20, lossRatio: 0.01}
    readTrick := &testConnTrick{delayMs: 20, lossRatio: 0.01}
    writeTrick := &testConnTrick{delayMs: 50, lossRatio: 0.05}

    c := &testMockLossyConn{}
    nc := NewLossyConn(c, 1024, readTrick, writeTrick)

    count := 500
    countReadLoss := 0
    buf := make([]byte, 4096)

    for j := 0; j < readTrick.delayMs; j++ {
        if c.countFeed < count {
            c.feed()
        }
        time.Sleep(1 * time.Millisecond)
    }

    for i := 0; i < count; i++ {
        nc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        if c.countFeed < count {
            c.feed()
        }
        n, err := nc.Read(buf)
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
        nc.Write([]byte(strconv.Itoa(now)))
    }

    <- time.After(1 * time.Second)
    fmt.Printf("done, count=%d, countFeed=%d, countReadLoss=%d, countWrite=%d\n", count, c.countFeed, countReadLoss, c.countWrite)
}


