package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	evmtypes "github.com/smartcontractkit/chainlink/core/chains/evm/types"
	"github.com/smartcontractkit/chainlink/core/services"
)

type livenessChecker struct {
	pool *Pool

	ctx    context.Context
	cancel context.CancelFunc
	chDone chan struct{}
}

var _ services.Service = &livenessChecker{}

func newLivenessChecker(pool *Pool) *livenessChecker {
	ctx, cancel := context.WithCancel(context.Background())
	return &livenessChecker{pool, ctx, cancel, make(chan struct{})}
}

func (l *livenessChecker) Start() error {
	go l.loop()
	return nil
}

func (l *livenessChecker) Close() error {
	l.cancel()
	<-l.chDone
	return nil
}

func (l *livenessChecker) loop() {
	defer close(l.chDone)

	var wg sync.WaitGroup
	headCollection := &headsCollection{make(map[Node]int64), sync.RWMutex{}}
	listeners := make(map[Node]*nodeMonitor)
	wg.Add(len(l.pool.nodes))
	for _, n := range l.pool.nodes {
		ch := make(chan<- *evmtypes.Head)
		m := &nodeMonitor{ch, -1}
		listeners[n] = m
		go n.loop(l.ctx, wg, headsCollection)
	}

	wg.Wait()
}

type headsCollection struct {
	latestNums map[Node]int64
	mu         sync.RWMutex
}

func (h *headsCollection) setLatest(n Node, latestNum int64) {
	h.mu.Lock()
	defer h.mu.RUnlock()
	current := h.latestNums[n]
	if latestNum > current {
		h.latestNums[n] = latestNum
	}
}

func (h *headsCollection) getLatest() (latest int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, n := range h.latestNums {
		if n > latest {
			latest = n
		}
	}
	return
}

// const NoHeadsAlar
type nodeMonitor struct {
	node          Node
	latestHeadNum int64
}

// From fiews failover proxy: https://github.com/Fiews/ChainlinkEthFailover/blob/master/index.js
// const headersTimeout = process.env.HEADERS_TIMEOUT*1000 || 180000
const headersTimeout = 3 * time.Minute // TODO: Needs to be configurable
const headersCheckUpToDate = headersTimeout / 10

// headNumLagThreshold is the maximum amount of heads the node is allowed to be
// behind the latest head from all nodes before it is marked dead
const headNumLagThreshold = 10

func (nm *nodeMonitor) loop(ctx context.Context, wg sync.WaitGroup, h *headsCollection) {
	defer wg.Done()
	ch := make(chan *evmtypes.Head)
	sub, err := nm.node.EthSubscribe(ctx, ch, "newHeads")
	if err != nil {
		panic("TODO", sub)
	}

	t1 := time.NewTicker(headersTimeout)
	t2 := time.NewTicker(headersCheckUpToDate)

	var latest int64

	for {
		select {
		case <-ctx.Done():
			return

		case blockHeader, open := <-ch:
			if !open {
				panic("TODO")
			}
			num := blockHeader.Number
			if num > latest {
				h.setLatest(nm.node, num)
			}
			t1.Reset(headersTimeout)

		case err, open := <-sub.Err():
			if open && err != nil {
				// TODO: Put it into the revive loop
				// TODO: DeclareDead(reason)
				// TODO: What if all the nodes are dead?
				nm.node.DeclareDead()
			}
		case <-t1.C:
			// We haven't received a head on the channel for the threshold, mark it dead
			panic("Node is dead, mark it")
		case <-t2.C:
			latestOfAll := h.getLatest()
			if latest < latestOfAll-headNumLagThreshold {
				nm.node.DeclareOutOfSync()
			}
		}
	}
}

// func (l *livenessChecker) Listen
//     defer done()
//     defer hl.unsubscribe()

//     ctx, cancel := utils.ContextFromChan(hl.chStop)
//     defer cancel()

// func (hl *headListener) subscribeToHead(ctx context.Context) error {
//     hl.chHeaders = make(chan *evmtypes.Head)

//     var err error
//     hl.headSubscription, err = hl.ethClient.SubscribeNewHead(ctx, hl.chHeaders)
//     if err != nil {
//         close(hl.chHeaders)
//         return errors.Wrap(err, "EthClient#SubscribeNewHead")
//     }

//     hl.connected.Store(true)

//     return nil

//     for {
//         if !hl.subscribe(ctx) {
//             break
//         }
//         err := hl.receiveHeaders(ctx, handleNewHead)
//         if ctx.Err() != nil {
//             break
//         } else if err != nil {
//             hl.logger.Errorw(fmt.Sprintf("Error in new head subscription, unsubscribed: %s", err.Error()), "err", err)
//             continue
//         } else {
//             break
//         }
//     }
// }
