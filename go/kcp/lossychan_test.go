package kcp

import (
    "fmt"
    "testing"
    "time"
    "sync"
)


type testChanTrick struct { }

func (t *testChanTrick) DelayMs() int {
    return 200
}

func (t *testChanTrick) LossRatio() float64 {
    return 0.3
}

func (t *testChanTrick) LimitPerSecond() int {
    return 100
}

func TestLossyChannel(test *testing.T) {
    sz := 32
    trick := &testChanTrick{}

    chOrig := make(chan interface{}, sz)
    chHijacked := LossyChannel("", chOrig, sz, trick)

    recvCount := 0;
    wg := sync.WaitGroup{}
    wg.Add(1)
    go func() {
        for {
            v := <- chHijacked
            if v == nil {
                println("hijacked closed")
                break
            }
            recvCount++;
            tm := v.(time.Time)

            fmt.Printf("delay = %d us\n", int(time.Now().Sub(tm).Seconds() * 1000000))
        }
        wg.Done()
    }()

    count := 200
    for i := 0; i < count; i++ {
        chOrig <- time.Now()

        delay := time.Second / time.Duration(trick.LimitPerSecond())
        time.Sleep(delay / 2) // twice faster, remote should only recv half
    }
    close(chOrig)
    wg.Wait()
    fmt.Printf("TestLossyChannel count = %d, recvCount = %d\n", count, recvCount)
}
