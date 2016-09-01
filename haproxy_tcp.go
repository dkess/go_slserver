package main

import (
	"log"
	"net"
)

func haproxyRun(hub *Hub, addr string) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}

		if hub.phasedOut {
			conn.Write([]byte("drain\n"))
		}

		conn.Close()
	}
}
