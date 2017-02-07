#include "ncmconn.h"
#include <event2/buffer.h>
#include <event2/event.h>


//FIXME: 选择一个合适的缓冲区大小
#define OUTBUF_LIMIT (20 * 1024)

NcmConn::NcmConn(struct event_base *evbase) {
    this->evbase = evbase;
    inputBuffer = evbuffer_new();
    outputBuffer = evbuffer_new();
}

NcmConn::~NcmConn() {
    evbuffer_free(inputBuffer);
    inputBuffer = nullptr;

    evbuffer_free(outputBuffer);
    outputBuffer = nullptr;
}


size_t NcmConn::inputBufferLength() {
    return evbuffer_get_length(inputBuffer);
}

size_t NcmConn::outputBufferLength() {
    return evbuffer_get_length(outputBuffer);
}

ssize_t NcmConn::outputFreeSpace() {
    return OUTBUF_LIMIT - evbuffer_get_length(outputBuffer);
}

void NcmConn::setEventCallback(const EventCallbackT &cb) {
    this->eventCallback = cb;
}

void NcmConn::doEventCallback(Event event, int err, size_t size) {
    if(!isClosedInternal) {
        eventCallback(this, event, err, size);
    }
}

bool NcmConn::isClosed() {
    return isClosedInternal;
}

void NcmConn::close() {
    if(!isClosedInternal) {
        isClosedInternal = true;
        doClose();
    }
}

bool NcmConn::isClosable() {
    return (!isClosedInternal && outputBufferLength() == 0);
}
