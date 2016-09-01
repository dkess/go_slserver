package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

func adminRun(hub *Hub, addr string) {
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

		go handleClient(conn, hub)
	}
}

func handleClient(conn io.ReadWriter, hub *Hub) {
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString(byte('\n'))
		msg = strings.TrimRight(msg, "\r\n")
		if err != nil {
			break
		}

		if msg == "snapshots" {
			snapshots := hub.getCoopSnapshots()
			conn.Write([]byte(strconv.Itoa(len(snapshots)) + "\n"))
			for _, snap := range snapshots {
				conn.Write([]byte(
					strconv.Itoa(snap.wordsGuessed) + " " + snap.word + " "))

				playerMsgs := make([][]byte, len(snap.players))
				for n, p := range snap.players {
					pMsg := make([]byte, len(p.name), len(p.name)+1)
					copy(pMsg, p.name)
					if !p.present {
						pMsg = append(pMsg, byte('_'))
					}
					playerMsgs[n] = pMsg
				}

				conn.Write(bytes.Join(playerMsgs, []byte(" ")))
				conn.Write([]byte("\n"))
			}
		} else if msg == "phaseout" {
			hub.phaseOutHub()
		}
	}
}
