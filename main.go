package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var url_re = regexp.MustCompile(`^/sixletters/ws/(?:(hostcoop|hostcomp)/([a-zA-Z0-9]{1,10})|join/(c|m)([a-zA-Z0-9]{5,8}))$`)

type FnSendText func([]byte)

// Parses a URL for its action.  If this is an invalid URL, the first bool will
// be true.  If this is a coop game, the second bool will be true.  If the user
// wants to host a game, the third bool will be true and the string will be the
// user's name.  If the user wants to join a game, the third bool will be false
/// and the string will be the desired game's name.
func parse_url(url string) (bool, bool, bool, string) {
	match := url_re.FindStringSubmatch(url)
	if len(match) == 0 {
		return true, false, false, ""
	}
	if match[1] == "hostcoop" {
		return false, true, true, match[2]
	} else if match[1] == "hostcomp" {
		return false, false, true, match[2]
	} else if match[3] == "c" {
		return false, true, false, match[4]
	} else if match[3] == "m" {
		return false, false, false, match[4]
	}
	return true, false, false, ""
}

func main() {
	debugFlag := flag.Bool("debug", false, "Run the program in debug mode")
	allowedHostFlag := flag.String("host", "", "The HTTP host to allow")
	prefixFlag := flag.String("prefix",
		"",
		"Text to prefix every game name on this server")
	addrFlag := flag.String("addr", ":8754", "The host and port to bind to")

	adminFlag := flag.String("admin-addr", "", "TCP socket for admin operations")
	haproxyAgentFlag := flag.String(
		"haproxy-agent-addr",
		"",
		"TCP socket for haproxy check-agent")

	flag.Parse()

	var upgrader = websocket.Upgrader{}
	if *debugFlag {
		fmt.Println("debug mode on")
		// When in debug mode, allow connections form all origins
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	if *allowedHostFlag != "" {
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return r.Host == *allowedHostFlag
		}
	}

	hub := newHub(*prefixFlag)
	go hub.run()

	if *adminFlag != "" {
		go adminRun(hub, *adminFlag)
	}

	if *haproxyAgentFlag != "" {
		go haproxyRun(hub, *haproxyAgentFlag)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail, isCoop, isHost, data := parse_url(r.URL.Path)
		if fail {
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		if err != nil {
			log.Println(err)
			return
		}

		if isCoop {
			var game *CoopGame
			var gamename string
			var pnum = 0
			if isHost {
				game = wsHostCoop(conn, data)
				if game == nil {
					return
				}
				gamename = hub.registerCoopGame(game)
				defer hub.leaveCoopGame(gamename)

				sendGameName(conn, gamename)
			} else {
				gamename = data
				game = hub.getCoopGame(gamename)
				if game == nil {
					conn.WriteMessage(websocket.TextMessage, []byte(":noexist"))
					return
				}
				defer hub.leaveCoopGame(gamename)
				pnum = wsJoinCoop(conn, game)
				if pnum < 0 {
					return
				}
			}

			wsCoopLoop(conn, game, pnum)
		} else {
			return
		}
	})

	fmt.Println("server started")
	err := http.ListenAndServe(*addrFlag, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func makeSendFunc(conn *websocket.Conn) FnSendText {
	return func(msg []byte) {
		sendText(conn, msg)
	}
}

func sendText(conn *websocket.Conn, msg []byte) {
	conn.WriteMessage(websocket.TextMessage, msg)
}
