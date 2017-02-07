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

func TestLossyChannel(test *testing.T) {
    sz := 32
    trick := &testChanTrick{}

    chOrig := make(chan interface{}, sz)
    chHijacked := LossyChannel(chOrig, sz, trick)

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

    tm := time.Now()
    count := 20000
    for i := 0; i < count; i++ {
        chOrig <- time.Now()
        fmt.Printf("send = %d us\n", int(time.Now().Sub(tm).Seconds() * 1000000)) //make same cpu usage as the recv routine
    }
    close(chOrig)
    wg.Wait()
    fmt.Printf("TestLossyChannel count = %d, recvCount = %d\n", count, recvCount)
}