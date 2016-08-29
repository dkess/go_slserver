package main

//import "fmt"

func newCPlayer(name string, send FnSendText) *CPlayer {
	return &CPlayer{
		name:   name,
		gaveup: false,
		send:   send,
	}
}

func newCoopGame(firstPlayer *CPlayer, wordsmap map[string]string) *CoopGame {
	return &CoopGame{
		wordsmap: wordsmap,
		players:  []*CPlayer{firstPlayer},

		playerjoin: make(chan *CPlayerJoinEvent),
		playerquit: make(chan *CPlayerQuitEvent),
		attempt:    make(chan *CAttemptEvent),
		giveup:     make(chan *CGiveupEvent),
	}
}

// Adds a player to the game, and returns the position that this player will
// be in, or -1 if this name was already taken.
func (g *CoopGame) playerJoin(name string, send FnSendText) int {
	result := make(chan int)
	g.playerjoin <- &CPlayerJoinEvent{
		name: name,
		send: send,
		pnum: result,
	}
	return <-result
}

// Called on a player disconnect.  Returns true if all players have quit.
func (g *CoopGame) playerQuit(pnum int) {
	g.playerquit <- &CPlayerQuitEvent{pnum: pnum}
}

func (g *CoopGame) wordAttempt(pnum int, word string) {
	g.attempt <- &CAttemptEvent{pnum: pnum, word: word}
}

func (g *CoopGame) playerGiveup(pnum int, didGiveUp bool) {
	g.giveup <- &CGiveupEvent{pnum: pnum, didGiveUp: didGiveUp}
}

func (g *CoopGame) run() {
	for {
		select {
		case pJoin := <-g.playerjoin:
			pnum := g.attemptJoinM(pJoin.name, pJoin.send)
			if pnum >= 0 {
				gameSendState(g, pnum)
			}
			pJoin.pnum <- pnum

		case pQuit := <-g.playerquit:
			g.players[pQuit.pnum].send = nil
			gameSendQuit(g, pQuit.pnum)

			g.checkGiveupM()

		case wordAttempt := <-g.attempt:
			word := wordAttempt.word
			if guesser, ok := g.wordsmap[word]; ok && guesser == "" {
				pname := g.players[wordAttempt.pnum].name
				g.wordsmap[word] = pname

				gameSendAttempt(g, wordAttempt.pnum, word)
			}

		case giveup := <-g.giveup:
			g.players[giveup.pnum].gaveup = giveup.didGiveUp
			gameSendGiveup(g, giveup.pnum, giveup.didGiveUp)

			g.checkGiveupM()
		}
	}
}

func (g *CoopGame) announceMsg(msg []byte, exclude int) {
	for n, p := range g.players {
		if n != exclude && p.send != nil {
			p.send(msg)
		}
	}
}

// Will immediately mutate the coop game and attempt to join this player.
// Should only be called from the game's main loop.  Will return the player
// number if successful, or -1 if the name was already taken.
func (g *CoopGame) attemptJoinM(name string, send FnSendText) int {
	for n, player := range g.players {
		if player.name == name {
			if player.send == nil {
				player.send = send
				return n
			} else {
				return -1
			}
		}
	}

	new_p := newCPlayer(name, send)
	g.players = append(g.players, new_p)
	return len(g.players) - 1
}

// Checks if all players have given up, and if they have, mutates the game
// state to fill in unguessed words with _, and sends the :allgiveup message to
// all clients.  If all players have disconnected, this will not trigger the
// allgiveup event-- at least one player must be present and giving up for this
// to happen.
func (g *CoopGame) checkGiveupM() {
	allGiveup := true
	allQuit := true
	for _, p := range g.players {
		if p.send != nil {
			allQuit = false
			if !p.gaveup {
				allGiveup = false
				break
			}
		}
	}

	if !allQuit && allGiveup {
		gameAllGiveup(g)
		for k, v := range g.wordsmap {
			if v == "" {
				g.wordsmap[k] = "_"
			}
		}
	}
}

type CoopGame struct {
	wordsmap map[string]string
	players  []*CPlayer

	playerjoin chan *CPlayerJoinEvent
	playerquit chan *CPlayerQuitEvent
	attempt    chan *CAttemptEvent
	giveup     chan *CGiveupEvent
}

type CPlayer struct {
	name   string
	gaveup bool
	send   FnSendText
}

type CPlayerJoinEvent struct {
	name string
	send FnSendText

	pnum chan int
}

type CPlayerQuitEvent struct {
	pnum int
}

type CAttemptEvent struct {
	pnum int
	word string
}

type CGiveupEvent struct {
	pnum      int
	didGiveUp bool
}
