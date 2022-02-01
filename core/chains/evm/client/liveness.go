package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	evmtypes "github.com/smartcontractkit/chainlink/core/chains/evm/types"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services"
)

// livenessChecker for now simply re-implements the exact same simple logic as
// the Fiews failover proxy which has been battle-tested for a long time
type livenessChecker struct {
	pool      *Pool
	deadAfter time.Duration

	logger logger.Logger
	ctx    context.Context
	cancel context.CancelFunc
	chDone chan struct{}
}

var _ services.Service = &livenessChecker{}

func NewLivenessChecker(pool *Pool, lggr logger.Logger, deadAfter time.Duration) *livenessChecker {
	lggr = lggr.Named("EVMLivenessChecker")
	ctx, cancel := context.WithCancel(context.Background())
	return &livenessChecker{pool, deadAfter, lggr, ctx, cancel, make(chan struct{})}
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

	if l.deadAfter == 0 {
		l.logger.Debug("Node dead after threshold set to 0; liveness checking is disabled")
		return
	}

	var wg sync.WaitGroup
	// Head subscription for each node
	// Mark broken if no heads for 3m
	// TODO: Add logic to handle case if there are no live nodes left
	wg.Add(len(l.pool.nodes))
	for _, n := range l.pool.nodes {
		go l.loopNode(wg, n)
	}

	wg.Wait()
}

func (l *livenessChecker) loopNode(wg sync.WaitGroup, node Node) {
	defer wg.Done()

	lggr := l.logger.With("node", node.String())

	ch := make(chan *evmtypes.Head)
	sub, err := node.EthSubscribe(l.ctx, ch, "newHeads")
	if err != nil {
		reason := fmt.Sprintf("initial subscription failed: %v", err)
		lggr.Warn("Node detected broken, removed from live nodes pool", "err", reason)
		node.DeclareBroken(reason, time.Now())
		return
	}
	defer func() {
		sub.Unsubscribe()
	}()
	lggr.Trace("successfully subscribed to heads feed")

	deadT := time.NewTicker(l.deadAfter)

	for {
		select {
		case <-l.ctx.Done():
			return

		case bh, open := <-ch:
			lggr.Trace("got head, resetting timer", "head", bh)
			if !open {
				reason := "subscription channel unexpectedly closed"
				lggr.Warn("Node detected broken, removed from live nodes pool", "err", reason)
				node.DeclareBroken(reason, time.Now())
				return
			}
			deadT.Reset(l.deadAfter)

		case err := <-sub.Err():
			if err != nil {
				// TODO: Put it into the revive loop
				// TODO: What if all the nodes are dead?
				reason := fmt.Sprintf("subscription errored: %v", err)
				lggr.Warn("Node detected broken, removed from live nodes pool", "err", reason)
				node.DeclareBroken(reason, time.Now())
				// exit loop
				return
			}
		case <-deadT.C:
			// We haven't received a head on the channel for at least the
			// threshold amount of time, mark it broken
			reason := fmt.Sprintf("no new heads received for %s", l.deadAfter)
			lggr.Warn("Node detected broken, removed from live nodes pool", "err", reason)
			node.DeclareBroken(reason, time.Now())
			return
		}
	}
}
