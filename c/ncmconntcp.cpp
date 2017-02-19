#include "ncmconntcp.h"
#include <errno.h>
#include <event2/event.h>
#include <event2/buffer.h>

//copied from libevent
#ifdef WIN32
#define NCM_EVUTIL_ERR_CONNECT_RETRIABLE(e) ((e) == WSAEWOULDBLOCK || (e) == WSAEINTR || (e) == WSAEINPROGRESS || (e) == WSAEINVAL)
#define NCM_EVUTIL_ERR_RW_RETRIABLE(e) ((e) == WSAEWOULDBLOCK || (e) == WSAEINTR)
#else
#define NCM_EVUTIL_ERR_CONNECT_RETRIABLE(e) ((e) == EINTR || (e) == EINPROGRESS)
#define NCM_EVUTIL_ERR_RW_RETRIABLE(e) ((e) == EINTR || (e) == EAGAIN)
#endif


#ifdef _DEBUG
    #define MAX_CONN_BUFFER_SIZE (800)
    #define MAX_RESPONSE_HEADER (700)
#else
    #define MAX_CONN_BUFFER_SIZE (16*1024)
    #define MAX_RESPONSE_HEADER (8*1024)

    #ifdef __APPLE__
        #include "TargetConditionals.h"
        #if TARGET_IPHONE_SIMULATOR
             // iOS Simulator
        #elif TARGET_OS_IPHONE
            //ios's memory is too little. do not use large buffer
            #undef MAX_CONN_BUFFER_SIZE
            #undef MAX_RESPONSE_HEADER
            #define MAX_CONN_BUFFER_SIZE (1500)
            #define MAX_RESPONSE_HEADER (1024)
        #elif TARGET_OS_MAC
            // Other kinds of Mac OS
        #else
            #error "Unknown Apple platform"
        #endif
    #endif
#endif


class NcmConnTcp::Internal {
public:
    int fd = -1;
    bool fdConnected = false;
    bool fdRemoteClosed = false;
    bool writeAsyncWaiting = false;

    size_t writtenSize = 0;
    int writeErrNo = 0;

    int writeOutputToSocket(NcmConnTcp *conn);

    struct event *eventFdReadable = nullptr;
    struct event *eventFdWritable = nullptr;
    static void evcbFdReadable(evutil_socket_t, short, void *);
    static void evcbFdWritable(evutil_socket_t, short, void *);
};

NcmConnTcp::NcmConnTcp(struct event_base *evbase) : NcmConn(evbase) {
    internal = new Internal();
}

NcmConnTcp::~NcmConnTcp() {
    close();
    delete internal;
    internal = nullptr;
}

void NcmConnTcp::doClose() {
    if (internal->fd != -1) {
        evutil_closesocket(internal->fd);
        internal->fd = -1;
    }

    if(internal->eventFdReadable != nullptr) {
        event_free(internal->eventFdReadable);
        internal->eventFdReadable = nullptr;
    }

    if(internal->eventFdWritable != nullptr) {
        event_free(internal->eventFdWritable);
        internal->eventFdWritable = nullptr;
    }
}

void NcmConnTcp::accept(int fd) {
    internal->fd = fd;
    internal->eventFdReadable = event_new(evbase, fd, EV_READ, Internal::evcbFdReadable, this);
    internal->eventFdWritable = event_new(evbase, fd, EV_WRITE, Internal::evcbFdWritable, this);
    internal->fdConnected = true;
}

void NcmConnTcp::connectAsync(const char *ipPort, int timeout) {
    sockaddr_storage ss;
    int ssLen = sizeof(ss);
    evutil_parse_sockaddr_port(ipPort, (sockaddr *) &ss, &ssLen);

    int fd = socket(AF_INET, SOCK_STREAM, 0);
    evutil_make_socket_nonblocking(fd);
    connect(fd, (sockaddr *) &ss, (socklen_t) ssLen);

    internal->fd = fd;
    internal->eventFdReadable = event_new(evbase, fd, EV_READ, Internal::evcbFdReadable, this);
    internal->eventFdWritable = event_new(evbase, fd, EV_WRITE, Internal::evcbFdWritable, this);

    struct timeval tv = {timeout, 0};
    event_add(internal->eventFdWritable, timeout == 0 ? nullptr : &tv);
}

void NcmConnTcp::readAsync() {
    if(isClosed() || internal->fdRemoteClosed)
        return;

    event_add(internal->eventFdReadable, nullptr);
}

int NcmConnTcp::Internal::writeOutputToSocket(NcmConnTcp *conn) {
    if(evbuffer_get_length(conn->outputBuffer) == 0) {
        return 0;
    }
    int n = evbuffer_write(conn->outputBuffer, fd);
    int e = (n >= 0) ? 0 : evutil_socket_geterror(fd);
    if(n < 0 && !NCM_EVUTIL_ERR_RW_RETRIABLE(e)) {
        fdRemoteClosed = true;
        writeErrNo = e;
        return n;
    }

    if(n <= 0) {
        return 0;
    } else {
        writtenSize += n;
        return n;
    }
}

void NcmConnTcp::writeAsync() {
    if(isClosed() || internal->fdRemoteClosed)
        return;

    internal->writeAsyncWaiting = true;

    //if not connected, we should wait for writable
    if(!internal->fdConnected) {
        event_add(internal->eventFdWritable, nullptr);
        return;
    }

    int n = internal->writeOutputToSocket(this);
    if (n < 0  /* error */ || outputFreeSpace() > 0 /* has space */ ) {
        event_active(internal->eventFdWritable, 0, 0);
    } else {
        // wait for socket writable
        event_add(internal->eventFdWritable, nullptr);
    }
}

void NcmConnTcp::Internal::evcbFdReadable(evutil_socket_t fd, short what, void *arg) {
    NcmConnTcp *conn = (NcmConnTcp *)arg;
    auto internal = conn->internal;
    auto len = evbuffer_read(conn->inputBuffer, fd, -1);

    if (len <= 0) {
        internal->fdRemoteClosed = true;
        int err = (len == 0) ? 0 : evutil_socket_geterror(fd);
        conn->doEventCallback(Event::Read, err, 0);
    } else {
        conn->doEventCallback(Event::Read, 0, (size_t)len);
    }
}

void NcmConnTcp::Internal::evcbFdWritable(evutil_socket_t fd, short what, void *arg) {
    NcmConnTcp *conn = (NcmConnTcp *)arg;
    auto internal = conn->internal;

    if(!internal->fdConnected) {
        if((what & EV_TIMEOUT) != 0) {
            conn->doEventCallback(Event::Connect, ETIMEDOUT, 0);
        } else {
            internal->fdConnected = true;
            conn->doEventCallback(Event::Connect, 0, 0);
        }
    }

    bool canDoCallback;
    if((what & EV_WRITE) != 0) {
        //activated by socket, or just connected, try to write more data
        int ret = internal->writeOutputToSocket(conn);
        if (ret >= 0 && conn->outputBufferLength()) {
            event_add(internal->eventFdWritable, nullptr);
        }

        canDoCallback = conn->outputFreeSpace() > 0;
        if(ret < 0) {
            //FIXME: what if write error ....
            canDoCallback = true;
        }
    } else {
        //activated by writeAsync because output buffer has free space, or error occurs
        //assert writeAsyncWaiting == true
        canDoCallback = true;
    }

    if (internal->writeAsyncWaiting && canDoCallback) {
        internal->writeAsyncWaiting = false;
        auto sz = internal->writtenSize;
        auto err = internal->writeErrNo;
        internal->writtenSize = 0;
        internal->writeErrNo = 0;
        conn->doEventCallback(Event::Write, err, sz);
    }
}
