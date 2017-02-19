#pragma once

#include <functional>

struct event_base;
class NcmProxy;

class NcmConn {
public:
    enum class Event : int {
        Connect = 1, Read = 2, Write = 3,
    };

    typedef std::function<void(NcmConn *conn, Event what, int err, size_t size)> EventCallbackT;

private:
    EventCallbackT eventCallback;
    bool isClosedInternal = false;

protected:
    NcmConn() = default;
    NcmConn(NcmConn const &) = delete;
    void operator=(NcmConn const &x) = delete;

    struct event_base *evbase;

    void doEventCallback(Event event, int err, size_t size);
    virtual void doClose() = 0;

public:
    struct evbuffer *inputBuffer = nullptr;
    struct evbuffer *outputBuffer = nullptr;

    size_t bytesRead = 0;
    size_t bytesWritten = 0;

    size_t inputBufferLength();
    size_t outputBufferLength();
    ssize_t outputFreeSpace();

    NcmConn(struct event_base *evbase);
    virtual ~NcmConn();

    void setEventCallback(const EventCallbackT &cb);

    bool isClosed();
    void close();

    virtual void connectAsync(const char *ipPort, int timeout) = 0;
    virtual void readAsync() = 0;
    virtual void writeAsync() = 0;
    bool isClosable();
};
