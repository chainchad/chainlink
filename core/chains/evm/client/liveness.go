package client

import (
	"fmt"
	"time"

	evmtypes "github.com/smartcontractkit/chainlink/core/chains/evm/types"
)

// livenessLoop for now simply re-implements the exact same simple logic as
// the Fiews failover proxy which has been battle-tested for a long time
//
// Should only run ONCE per node
func (n *node) livenessLoop() {
	defer n.wg.Done()

	ch := make(chan *evmtypes.Head)
	sub, err := n.EthSubscribe(n.ctx, ch, "newHeads")
	if err != nil {
		reason := fmt.Sprintf("initial subscription failed: %v", err)
		n.log.Warn("Node detected broken, removed from live nodes pool", "err", reason)
		n.DeclareBroken(reason, time.Now())
		return
	}
	defer func() {
		sub.Unsubscribe()
	}()
	n.log.Trace("successfully subscribed to heads feed")

	deadAfter := n.cfg.NodeDeadAfterNoNewHeadersThreshold()
	deadT := time.NewTicker(deadAfter)
	var latestReceivedBlock int64 = -1

	for {
		select {
		case <-n.ctx.Done():
			return

		case bh, open := <-ch:
			n.log.Trace("got head, resetting timer", "head", bh)
			if !open {
				reason := "subscription channel unexpectedly closed"
				n.log.Warn("Node detected broken, removed from live nodes pool", "err", reason)
				n.DeclareBroken(reason, time.Now())
				return
			}
			if bh.Number > latestReceivedBlock {
				latestReceivedBlock = bh.Number
			}
			deadT.Reset(deadAfter)

		case err := <-sub.Err():
			if err != nil {
				// TODO: Put it into the revive loop
				// TODO: What if all the nodes are dead?
				reason := fmt.Sprintf("subscription errored: %v", err)
				n.log.Warn("Node detected broken, removed from live nodes pool", "err", reason)
				n.DeclareBroken(reason, time.Now())
				// exit loop
				return
			}
		case <-deadT.C:
			// We haven't received a head on the channel for at least the
			// threshold amount of time, mark it broken
			reason := fmt.Sprintf("no new heads received for %s", deadAfter)
			n.log.Warn("Node detected out of sync, removed from live nodes pool", "err", reason)
			n.DeclareOutOfSync(latestReceivedBlock)
			n.wg.Add(1)
			go n.reviveLoop()
			return
		}
	}
}

// reviveLoop takes an OutOfSync node and puts it back to live status if it
// receives a head
func (n *node) reviveLoop() {
}
