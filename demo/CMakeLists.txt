cmake_minimum_required(VERSION 3.2.3)
project(kcpconn-demo)

include_directories("../c")

set(CMAKE_CXX_FLAGS "-std=c++11")

link_libraries("event")

FILE(GLOB SRCS
        *.cpp
        ../c/kcp/*.c
        ../c/*.cpp
        )

add_executable("kcpconn-demo" ${SRCS})
