#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <event2/event.h>
#include <event2/thread.h>
#include <event2/buffer.h>
#include "ncmconntcp.h"
#include "ncmconnkcp.h"

#define LOCAL_ADDR "127.0.0.1:8080"
#define KCP_HTTP_ADDR "127.0.0.1:8880"
#define KCP_HTTPS_ADDR "127.0.0.1:8883"

static void panic(const char *msg) {
    printf("panic: %s, errno=%d(%s)\n", msg, errno, strerror(errno));
    exit(1);
}

static void run(event_base *evbase) {
    NcmConnKcpManager manager(evbase);
    NcmConnKcp *kcp = manager.newKcpConn();;
    NcmConnTcp *tcp = new NcmConnTcp(evbase);

    bool clientHeadCompleted = false;

    manager.setCloseWaitMs(500);  //this is only a demo, we do not need to wait for too long

    sockaddr_storage ss;
    int ssLen = sizeof(ss);
    int fdTcpServer = socket(AF_INET, SOCK_STREAM, 0);
    if(fdTcpServer == -1) panic("can not create socket");

    evutil_parse_sockaddr_port(LOCAL_ADDR, (sockaddr *)&ss, &ssLen);
    if(bind(fdTcpServer, (sockaddr *)&ss, (socklen_t)ssLen) != 0) panic("can not bind");
    if(listen(fdTcpServer, 1) != 0) panic("can not listen");

    printf("waiting for request from %s ...\n", LOCAL_ADDR);
    socklen_t sl = sizeof(ss);
    int fd = accept(fdTcpServer, (sockaddr *)&ss, &sl);
    if(fd == -1) panic("failed to accept");

    tcp->setEventCallback([tcp, kcp, &clientHeadCompleted](NcmConn *conn, NcmConn::Event what, int err, size_t size) {
        printf("tcp event what=%d, err=%d, size=%ld\n", what, err, size);
        auto peer = kcp;
        if(what == NcmConn::Event::Connect) {
            conn->writeAsync();
        } if(what == NcmConn::Event::Write) {
            if (conn->outputBufferLength()) {
                conn->writeAsync();
            }
        } else if(what == NcmConn::Event::Read) {
            if(conn->inputBufferLength()) {
                if(!clientHeadCompleted) {
                    auto p = evbuffer_search(conn->inputBuffer, "\r\n\r\n", 4, NULL);
                    if(p.pos > 0) {
                        //http head is completed
                        clientHeadCompleted = true;

                        auto headSize = p.pos + 4;
                        char *head = new char[headSize + 1];
                        evbuffer_remove(conn->inputBuffer, head, (size_t)headSize);
                        head[headSize] = 0;
                        printf("got http request:\n%s\n", head);

                        //connect to peer
                        if(strncmp(head, "CONNECT", 7) == 0) {
                            peer->connectAsync(KCP_HTTPS_ADDR);

                            evbuffer_add_printf(conn->outputBuffer, "HTTP/1.0 200 Connection established\r\n");
                            evbuffer_add_printf(conn->outputBuffer, "Proxy-Agent: demo\r\n");
                            evbuffer_add_printf(conn->outputBuffer, "\r\n");
                            conn->writeAsync();
                        } else {
                            peer->connectAsync(KCP_HTTP_ADDR);
                            evbuffer_add(peer->outputBuffer, head, (size_t)headSize);
                            peer->writeAsync();
                        }

                        delete []head;
                        peer->readAsync();
                    }
                }

                if(clientHeadCompleted) {
                    evbuffer_add_buffer(peer->outputBuffer, conn->inputBuffer);
                    peer->writeAsync();
                }
            }

            if (size != 0) {
                conn->readAsync();
            } else {
                printf("tcp closed\n");
                conn->close();
                kcp->close();  // TODO: remote http server is doing http keep-alive. we just simply close it
            }
        }
    });

    kcp->setEventCallback([tcp, kcp](NcmConn *conn, NcmConn::Event what, int err, size_t size) {
        printf("kcp event what=%d, err=%d, size=%ld\n", what, err, size);
        auto peer = tcp;
        if(what == NcmConn::Event::Write) {
            if (conn->outputBufferLength()) {
                conn->writeAsync();
            } else {
                if(peer->isClosed()) {
                    conn->close();
                }
            }
        } else if(what == NcmConn::Event::Read) {
            if(conn->inputBufferLength()) {
                evbuffer_add_buffer(peer->outputBuffer, conn->inputBuffer);
                peer->writeAsync();
            }

            if (size != 0) {
                conn->readAsync();
            } else {
                printf("kcp closed\n");
                conn->close();
            }
        }
    });


    tcp->accept(fd);
    tcp->readAsync();

    //kcp may wait for 5 more seconds to do close wait
    event_base_dispatch(evbase);

    delete tcp;
    delete kcp;

    evutil_closesocket(fdTcpServer);
}

int main() {
    while(time(NULL)) {
        event_base *evbase = event_base_new();
        run(evbase);
        event_base_free(evbase);
        puts("\n-- over --\n");
    }
    return 0;
}
