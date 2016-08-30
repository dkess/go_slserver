package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/gorilla/websocket"
)

var wordsregexp = regexp.MustCompile(`[[:lower:]]{3,6}_?`)
var nameregexp = regexp.MustCompile(`^[a-zA-Z0-9]{1,10}$`)
var attemptregexp = regexp.MustCompile(`^:attempt ([a-z]{3,6})`)

type ClientCoop struct {
	game *CoopGame

	conn *websocket.Conn
	send chan []byte
}

// Takes a list of space-separated words, and returns a map.  Words followed
// by _s will be mapped to the guesser, otherwise they will be mapped to an
// empty string.  Returns nil on an invalid words string.
func makeWordsMap(words string, guesser string) map[string]string {
	var splitWords = strings.Split(words, " ")
	if len(splitWords) > 80 {
		return nil
	}
	var wordsmap = make(map[string]string)
	for _, word := range splitWords {
		if !wordsregexp.MatchString(word) {
			return nil
		}

		if word[len(word)-1] == '_' {
			wordsmap[word[:len(word)-1]] = guesser
		} else {
			wordsmap[word] = ""
		}
	}
	return wordsmap
}

func wsHostCoop(conn *websocket.Conn, pname string) *CoopGame {
	messageType, r, err := conn.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		return nil
	}
	wordsmap := makeWordsMap(string(r), pname)
	if wordsmap == nil {
		return nil
	}

	p := newCPlayer(pname, makeSendFunc(conn))

	game := newCoopGame(p, wordsmap)
	go game.run()

	return game
}

func sendGameName(conn *websocket.Conn, name string) {
	sendText(conn, []byte(name))
}

// Waits for the user to input a player name, and joins the game with that
// name.  Returns the player number, or -1 if the join failed.
func wsJoinCoop(conn *websocket.Conn, game *CoopGame) int {
	sendText(conn, []byte(":ok"))
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil || messageType != websocket.TextMessage {
			return -1
		}
		if nameregexp.Match(p) {
			pnum := game.playerJoin(string(p), makeSendFunc(conn))
			if pnum < 0 {
				sendText(conn, []byte(":taken"))
			} else {
				return pnum
			}
		} else {
			sendText(conn, []byte(":badname"))
		}
	}

	return -1
}

// The game loop.  Returns when this player disconnects.  If returns true, all
// players have left.
func wsCoopLoop(conn *websocket.Conn, game *CoopGame, pnum int) {
	defer func() {
		recover()
		game.playerQuit(pnum)
	}()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil || messageType != websocket.TextMessage {
			break
		}

		match := attemptregexp.FindSubmatch(p)
		if len(match) != 0 {
			word := match[1]
			game.wordAttempt(pnum, string(word))
		} else if bytes.Equal(p, []byte(":giveup")) {
			game.playerGiveup(pnum, true)
		} else if bytes.Equal(p, []byte(":ungiveup")) {
			game.playerGiveup(pnum, false)
		}
	}
	panic("broke out of ws read loop")
}

// These functions, prefixed with the word "game," are called directly from
// inside the game loop, and are therefore allowed to access game fields.
// They are called whenever a game event happens and clients need to be
// notified of the event.

// Sends the game state to a player who has just joined the game, and announces
// to other players that the player has joined.  `newpnum` is the player number
// of the recently joined player.
func gameSendState(game *CoopGame, newpnum int) {
	newp := game.players[newpnum]
	// announce to everyone else that this player has joined
	joinMsg := []byte(fmt.Sprintf(":join %s", newp.name))
	game.announceMsg(joinMsg, newpnum)

	// send this player the player list
	playerMsgs := make([][]byte, len(game.players))
	for n, p := range game.players {
		pMsg := make([]byte, len(p.name), len(p.name)+1)
		copy(pMsg, p.name)
		if p.send == nil {
			pMsg = append(pMsg, byte('_'))
		}
		playerMsgs[n] = pMsg
	}
	newp.send(bytes.Join(playerMsgs, []byte(" ")))

	// send the list of words
	wordMsgs := make([][]byte, len(game.wordsmap))
	var n = 0
	for word, _ := range game.wordsmap {
		wordMsgs[n] = []byte(word)
		n++
	}
	newp.send(bytes.Join(wordMsgs, []byte(" ")))

	// send guessers of previously guessed words
	for word, guesser := range game.wordsmap {
		if guesser != "" {
			var attemptMsg = fmt.Sprintf(":attempt %s %s", word, guesser)
			newp.send([]byte(attemptMsg))
		}
	}

	for _, p := range game.players {
		if p.gaveup {
			var msg = fmt.Sprintf(":giveup %s", p.name)
			newp.send([]byte(msg))
		}
	}
}

// Called when a player has quit the game.
func gameSendQuit(game *CoopGame, pnum int) {
	msg := []byte(fmt.Sprintf(":quit %s", game.players[pnum].name))
	game.announceMsg(msg, pnum)
}

// Called when a player correctly guesses a word.
func gameSendAttempt(game *CoopGame, pnum int, word string) {
	msg := []byte(fmt.Sprintf(":attempt %s %s", word, game.players[pnum].name))
	game.announceMsg(msg, pnum)
}

// Called when a player gives up, or removes their vote to give up.
func gameSendGiveup(game *CoopGame, pnum int, didGiveUp bool) {
	var msg string
	if didGiveUp {
		msg = fmt.Sprintf(":giveup %s", game.players[pnum].name)
	} else {
		msg = fmt.Sprintf(":ungiveup %s", game.players[pnum].name)
	}
	game.announceMsg([]byte(msg), pnum)
}

// Called when all players have given up.
func gameAllGiveup(game *CoopGame) {
	game.announceMsg([]byte(":allgiveup"), -1)
}
