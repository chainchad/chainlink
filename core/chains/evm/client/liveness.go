package client

import (
	"context"

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

	listeners := make(map[Node]chan<- *evmtypes.Head)
	for _, n := range l.pool.nodes {
		ch := make(chan<- *evmtypes.Head)
		listeners[n] = ch
		if sub, err := n.EthSubscribe(context.TODO(), ch, "newHeads"); err != nil {
			panic("TODO", sub)
		}
	}

	for {
		select {}
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
