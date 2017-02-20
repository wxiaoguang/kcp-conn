#include "ncmconnkcp.h"
#include "kcp/ikcp.h"
#include <event2/event.h>
#include <event2/buffer.h>
#include <errno.h>
#include <list>

#define KCP_CLOSE_WAIT_MS 5000

/*
 * C <- S: close
 * C -> S: close
 * C: close now, S: wait for a few seconds
 */

class NcmConnKcpManager::Internal {
public:
    std::list<NcmConnKcp::Internal *> connInternals;

    struct event_base *evbase;
    uint32_t closeWaitMs = KCP_CLOSE_WAIT_MS;
    void closeConn(NcmConnKcpManager *manager, NcmConnKcp *conn);
    void deleteConnInternal(NcmConnKcpManager *manager, NcmConnKcp::Internal *connInternal);
};

class NcmConnKcp::Internal {
public:
    NcmConnKcp *conn;
    NcmConnKcpManager *manager;

    int fd = -1;
    ikcpcb *kcp = nullptr;
    uint32_t connectStartTimeMs = 0;
    uint32_t connectTimeoutMs = 0;

    struct event *eventFdReadable = nullptr;

    struct event *eventKcpUpdate = nullptr;
    struct event *eventKcpReadWrite = nullptr;

    bool readAsyncWaiting = false;
    bool writeAsyncWaiting = false;
    uint32_t closeTimeMs = 0;
    uint32_t updateDelayMs = 0;

    int lastErrNo = 0;
    size_t readSize = 0;
    size_t writtenSize = 0;

    bool isSndQueueWritable();
    void scheduleNextUpdate();

    void feedInputBufferReadable(NcmConnKcp *conn);
    void feedOutputBufferWritable(NcmConnKcp *conn);

    static int kcpOutputWrapper(const char *data, int len, struct IKCPCB *kcp, void *user);

    static void evcbFdReadable(evutil_socket_t, short, void *);
    static void evcbKcpUpdate(evutil_socket_t, short, void *);
    static void evcbKcpReadWrite(evutil_socket_t, short, void *);

    ~Internal(){
        manager = nullptr;

        if (fd != -1) {
            evutil_closesocket(fd);
            fd = -1;
        }

        if (kcp) {
            ikcp_release(kcp);
            kcp = nullptr;
        }

        if(eventFdReadable != nullptr) {
            event_free(eventFdReadable);
            eventFdReadable = nullptr;
        }

        if(eventKcpUpdate != nullptr) {
            event_free(eventKcpUpdate);
            eventKcpUpdate = nullptr;
        }

        if(eventKcpReadWrite != nullptr) {
            event_free(eventKcpReadWrite);
            eventKcpReadWrite = nullptr;
        }

    }
};

//FIXME: consider monotonic clock
static inline uint32_t currentMs() {
    struct timeval time;
    gettimeofday(&time, NULL);
    return uint32_t((time.tv_sec * 1000) + (time.tv_usec / 1000));
}

NcmConnKcp::NcmConnKcp(NcmConnKcpManager *manager, uint32_t conversationId) : NcmConn(manager->internal->evbase) {
    internal = new Internal();

    evbase = manager->internal->evbase;

    internal->conn = this;
    internal->manager = manager;
    internal->eventKcpUpdate = event_new(evbase, -1, 0, Internal::evcbKcpUpdate, internal);
    internal->eventKcpReadWrite = event_new(evbase, -1, 0, Internal::evcbKcpReadWrite, internal);

    internal->kcp = ikcp_create(conversationId, internal);
    if(internal->kcp) {
        internal->kcp->output = Internal::kcpOutputWrapper;
        internal->kcp->stream = 1;
    }
}

NcmConnKcp::~NcmConnKcp() {
    close();
}

void NcmConnKcp::setKcpMtu(int mtu) {
    ikcp_set_mtu(internal->kcp, mtu);
}

void NcmConnKcp::setKcpOptions(int noDelay, int interval, int resend, int noCwnd) {
    /*
       - nodelay ：是否启用 nodelay模式，0不启用；1启用。
       - interval ：协议内部工作的 interval，单位毫秒，比如 10ms或者 20ms
       - resend ：快速重传模式，默认0关闭，可以设置2（2次ACK跨越将会直接重传）
       - nc ：是否关闭流控，默认是0代表不关闭，1代表关闭。
       - 普通模式：`ikcp_nodelay(kcp, 0, 40, 0, 0);
       - 极速模式： ikcp_nodelay(kcp, 1, 10, 2, 1);
     */
    ikcp_set_nodelay(internal->kcp, noDelay, interval, resend, noCwnd);
}

void NcmConnKcp::setKcpWnd(int sndWnd, int rcvWnd) {
    ikcp_set_wnd(internal->kcp, sndWnd, rcvWnd);
}

void NcmConnKcp::connectAsync(const char *ipPort, int timeout) {
    sockaddr_storage ss;
    int ssLen = sizeof(ss);
    evutil_parse_sockaddr_port(ipPort, (sockaddr *) &ss, &ssLen);

    int fd = socket(AF_INET, SOCK_DGRAM, 0);
    evutil_make_socket_nonblocking(fd);
    connect(fd, (sockaddr *) &ss, (socklen_t) ssLen);

    internal->connectStartTimeMs = currentMs();
    internal->connectTimeoutMs = (uint32_t)timeout * 1000;
    internal->fd = fd;
    internal->eventFdReadable = event_new(evbase, fd, EV_READ | EV_PERSIST, Internal::evcbFdReadable, internal);
    event_add(internal->eventFdReadable, NULL);

    ikcp_update(internal->kcp, currentMs());
    ikcp_send_connect_flush(internal->kcp);

    internal->scheduleNextUpdate();
}

void NcmConnKcp::readAsync() {
    if(!internal) return;

    internal->readAsyncWaiting = true;
    internal->feedInputBufferReadable(this);
}

void NcmConnKcp::writeAsync() {
    if(!internal) return;

    internal->writeAsyncWaiting = true;
    internal->feedOutputBufferWritable(this);
}

void NcmConnKcp::doClose() {
    if(!internal) return;

    ikcp_send_close_flush(internal->kcp);

    // prevent zero (zero means remote not closed)
    internal->closeTimeMs = internal->closeTimeMs ? internal->closeTimeMs : (currentMs() | 1);
    internal->manager->internal->closeConn(internal->manager, this);
}

bool NcmConnKcp::Internal::isSndQueueWritable() {
    return ikcp_waitsnd(kcp) < kcp->snd_wnd * 2;
}

int NcmConnKcp::Internal::kcpOutputWrapper(const char *data, int len, struct IKCPCB *, void *user) {
    NcmConnKcp::Internal *internal = (NcmConnKcp::Internal *)user;

    //we do not care about failure here.
    ssize_t n = send(internal->fd, data, (size_t)len, 0);

    return (int)n;
}

void NcmConnKcp::Internal::scheduleNextUpdate() {
    if (updateDelayMs >= 1000) updateDelayMs = 1000;
    if (updateDelayMs < kcp->interval) updateDelayMs = kcp->interval;

    struct timeval tv = {0, (int)updateDelayMs * 1000};
    event_add(eventKcpUpdate, &tv);
}

void NcmConnKcp::Internal::feedInputBufferReadable(NcmConnKcp *conn) {

    //only read when we do a read request
    if(!readAsyncWaiting)
        return;

    //read kcp buffer to our input buffer
    while(true) {
        int sz = ikcp_recv_size(kcp);
        if(sz <= 0) break;

        struct evbuffer_iovec iv[1];
        evbuffer_reserve_space(conn->inputBuffer, sz, iv, 1);
        ikcp_recv(kcp, (char *)iv[0].iov_base, sz);
        iv[0].iov_len = (size_t)sz;
        evbuffer_commit_space(conn->inputBuffer, iv, 1);
        readSize += sz;
    }

    if (readSize) {
        readAsyncWaiting = false;
        event_active(eventKcpReadWrite, EV_READ, 0);
    } else {
        if (ikcp_should_close(kcp)) {

            //assert readSize == 0

            ikcp_send_close_flush(kcp);

            // prevent zero (zero means remote not closed)
            closeTimeMs = closeTimeMs ? closeTimeMs : (currentMs() | 1);
            readAsyncWaiting = false;
            event_active(eventKcpReadWrite, EV_READ, 0);
        } else if (ikcp_is_all_open(kcp)) {
            // wait to recv more data
        } else {
            // FIXME: read after closed? dead? what to do?
        }
    }
}

void NcmConnKcp::Internal::feedOutputBufferWritable(NcmConnKcp *conn) {
    if(conn->isClosed())
        return;

    //write as much as possible
    size_t sent = 0;
    if(isSndQueueWritable()) {
        size_t len = evbuffer_get_length(conn->outputBuffer);
        if(len > 0) {
            int nvec = evbuffer_peek(conn->outputBuffer, -1, NULL, NULL, 0);
            evbuffer_iovec eviovec[nvec];
            evbuffer_peek(conn->outputBuffer, -1, NULL, eviovec, nvec);
            for(int i = 0; i < nvec; i++) {
                if (eviovec[i].iov_len > 0) {
                    ikcp_send(kcp, (char *)eviovec[i].iov_base, (int)eviovec[i].iov_len);
                    sent += eviovec[i].iov_len;
                    if(!isSndQueueWritable()) break;
                }
            }
            evbuffer_drain(conn->outputBuffer, sent);

            if(ikcp_sndbuf_avail(kcp) > 0) {
                ikcp_flush(kcp);
            }
        }
    }
    writtenSize += sent;

    if(writeAsyncWaiting) {
        if(conn->outputFreeSpace() > 0) {
            writeAsyncWaiting = false;
            event_active(eventKcpReadWrite, EV_WRITE, 0);
        } else {
            //wait for kcp send queue to be consumed
        }
    }
}

void NcmConnKcp::Internal::evcbFdReadable(evutil_socket_t fd, short what, void *arg) {
    NcmConnKcp::Internal *internal = (NcmConnKcp::Internal *)arg;
    auto conn = internal->conn;
    auto kcp = internal->kcp;

    //recv from udp socket, do kcp input

    int alreadyConnected = ikcp_state_connected(internal->kcp);
    int count = 0;
    while(true) {
        uint8_t buf[2048];
        ssize_t n = recv(fd, buf, sizeof(buf), 0);
        if(n <= 0) break;
        ikcp_input(kcp, (char *)buf, n);
        count++;
    }

    //if no data received, there must be an error
    if(count == 0) {
        //FIXME: dead .....
        internal->lastErrNo = evutil_socket_geterror(fd);
        ikcp_set_state_dead(kcp);
    }

    if(!alreadyConnected && ikcp_state_connected(kcp)) {
        ikcp_send_connect_flush(kcp); //if we are the server, should reply a connect packet
        if(conn) {
            conn->doEventCallback(Event::Connect, 0, 0);
        }
    }

    ikcp_update(kcp, currentMs());
    if(conn) {
        internal->feedInputBufferReadable(conn);
        internal->feedOutputBufferWritable(conn);
    }

    internal->updateDelayMs /= 2;
    internal->scheduleNextUpdate();
}

void NcmConnKcp::Internal::evcbKcpUpdate(evutil_socket_t fd, short what, void *arg) {
    NcmConnKcp::Internal *internal = (NcmConnKcp::Internal *)arg;
    auto conn = internal->conn;
    auto kcp = internal->kcp;
    auto manager = internal->manager;

    auto now = currentMs();
    ikcp_update(kcp, now);

    //printf("update: delay=%d, inputBufLen=%ld, outBufLen=%ld, nsnd_buf=%d, nsnd_que=%d, nrcv_buf=%d, nrcv_que=%d\n", internal->updateDelayMs, conn->inputBufferLength(), conn->outputBufferLength(), kcp->nsnd_buf, kcp->nsnd_que, kcp->nrcv_buf, kcp->nrcv_que);
    //printf("update: snd_nxt=%d, snd_una=%d, rcv_nxt=%d, \n", kcp->snd_nxt, kcp->snd_una, kcp->rcv_nxt);
    if(conn) {
        //connection in use
        if(internal->connectTimeoutMs != 0 && (now - internal->connectStartTimeMs > internal->connectTimeoutMs)) {
            if(!ikcp_state_connected(kcp)) {
                conn->doEventCallback(Event::Connect, ETIMEDOUT, 0);
                internal->connectTimeoutMs = 0;
            }
        }
    } else {
        //close waiting
        if (currentMs() - internal->closeTimeMs >= manager->internal->closeWaitMs) {
            //should be destroyed
            internal->manager->internal->deleteConnInternal(internal->manager, internal);
            return;
        }
    }

    internal->updateDelayMs *= 2;
    internal->scheduleNextUpdate();
}

void NcmConnKcp::Internal::evcbKcpReadWrite(evutil_socket_t fd, short what, void *arg) {
    NcmConnKcp::Internal *internal = (NcmConnKcp::Internal *)arg;
    auto conn = internal->conn;

    if((what & EV_READ) != 0) {
        auto err = internal->lastErrNo;
        auto sz = internal->readSize;
        internal->lastErrNo = 0;
        internal->readSize = 0;
        conn->doEventCallback(Event::Read, err, sz);
    } else if((what & EV_WRITE) != 0) {
        auto err = internal->lastErrNo;
        auto sz = internal->writtenSize;
        internal->lastErrNo = 0;
        internal->writtenSize = 0;
        conn->doEventCallback(Event::Write, err, sz);
    }
}





NcmConnKcpManager::NcmConnKcpManager(struct event_base *evbase) {
    internal = new Internal();
    internal->evbase = evbase;

    evutil_secure_rng_init();
}

NcmConnKcpManager::~NcmConnKcpManager() {
    while(!internal->connInternals.empty()) {
        internal->deleteConnInternal(this, internal->connInternals.front());
    }
    delete internal;
}

NcmConnKcp *NcmConnKcpManager::newKcpConn() {
    uint32_t conv;
    evutil_secure_rng_get_bytes(&conv, 4);
    NcmConnKcp *conn = new NcmConnKcp(this, conv);
    return conn;
}

void NcmConnKcpManager::setCloseWaitMs(uint32_t n) {
    internal->closeWaitMs = n;
}

void NcmConnKcpManager::Internal::closeConn(NcmConnKcpManager *manager, NcmConnKcp *conn) {
    auto connInternal = conn->internal;
    conn->internal = nullptr;
    connInternal->conn = nullptr;

    event_free(connInternal->eventKcpReadWrite);
    connInternal->eventKcpReadWrite = nullptr;

    connInternals.push_back(connInternal);
}

void NcmConnKcpManager::Internal::deleteConnInternal(NcmConnKcpManager *manager, NcmConnKcp::Internal *connInternal) {
    connInternals.remove(connInternal);
    delete connInternal;
}
