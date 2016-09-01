package main

import (
	"math/rand"
	"os"
	"time"
)

// The amount of time to wait to erase a game after all of its players
// have left.
const KILL_TIME = time.Hour * time.Duration(24)

func newHub(prefix string) *Hub {
	return &Hub{
		coopGames: make(map[string]*CoopGameEntry),
		prefix:    prefix,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),

		registerCoop: make(chan *RegisterCoopGame),
		getCoop:      make(chan *GetCoopGame),
		leaveCoop:    make(chan string),
		removeCoop:   make(chan string),

		getCSnapshots: make(chan chan<- []*CGameSnapshot),
		phaseout:      make(chan bool),
	}
}

func (h *Hub) registerCoopGame(game *CoopGame) string {
	nameChan := make(chan string)
	r := &RegisterCoopGame{game: game, name: nameChan}
	h.registerCoop <- r
	return <-nameChan
}

func (h *Hub) getCoopGame(name string) *CoopGame {
	gameChan := make(chan *CoopGame)
	j := &GetCoopGame{game: gameChan, name: name}
	h.getCoop <- j
	return <-gameChan
}

func (h *Hub) leaveCoopGame(name string) {
	h.leaveCoop <- name
}

func (h *Hub) getCoopSnapshots() []*CGameSnapshot {
	s := make(chan []*CGameSnapshot)
	h.getCSnapshots <- s
	return <-s
}

func (h *Hub) phaseOutHub() {
	h.phaseout <- true
}

// Generates a string with random lowercase alphanumeric chars
func generateGamename(rng *rand.Rand) string {
	var gamename_b [5]byte
	for i := 0; i < len(gamename_b); i++ {
		r := rng.Intn(36)
		if r < 10 {
			gamename_b[i] = byte('0' + r)
		} else {
			gamename_b[i] = byte('a' + r - 10)
		}
	}
	return string(gamename_b[:])
}

func (h *Hub) run() {
	for {
		select {
		case rCoop := <-h.registerCoop:
			var gamename = h.prefix + generateGamename(h.rng)
			for {
				if _, exists := h.coopGames[gamename]; !exists {
					break
				}
				gamename = h.prefix + generateGamename(h.rng)
			}
			killTimer := time.AfterFunc(KILL_TIME, func() {
				h.removeCoop <- gamename
			})
			killTimer.Stop()
			h.coopGames[gamename] = &CoopGameEntry{
				game:        rCoop.game,
				killTimer:   killTimer,
				connections: 1,
			}
			rCoop.name <- gamename

		case gCoop := <-h.getCoop:
			game, prs := h.coopGames[gCoop.name]
			if !prs {
				gCoop.game <- nil
				continue
			}
			game.connections += 1
			game.killTimer.Stop()
			gCoop.game <- h.coopGames[gCoop.name].game

		case lCoop := <-h.leaveCoop:
			game := h.coopGames[lCoop]
			game.connections -= 1
			if game.connections == 0 {
				game.killTimer.Reset(KILL_TIME)
			}

		case kCoop := <-h.removeCoop:
			delete(h.coopGames, kCoop)
			if len(h.coopGames) == 0 {
				os.Exit(0)
			}

		case gs := <-h.getCSnapshots:
			snapshots := make([]*CGameSnapshot, len(h.coopGames))
			n := 0
			for _, ge := range h.coopGames {
				snapshots[n] = ge.game.getSnapshot()
			}

			gs <- snapshots

		case <-h.phaseout:
			h.phasedOut = true
			if len(h.coopGames) == 0 {
				os.Exit(0)
			}
		}
	}
}

type CoopGameEntry struct {
	game        *CoopGame
	killTimer   *time.Timer
	connections int
}

type Hub struct {
	coopGames map[string]*CoopGameEntry
	prefix    string
	phasedOut bool

	rng *rand.Rand

	registerCoop chan *RegisterCoopGame
	getCoop      chan *GetCoopGame
	leaveCoop    chan string
	removeCoop   chan string
	phaseout     chan bool

	getCSnapshots chan chan<- []*CGameSnapshot
}

type RegisterCoopGame struct {
	game *CoopGame
	name chan string
}

type GetCoopGame struct {
	name string
	game chan *CoopGame
}
