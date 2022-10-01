package impl

import (
	"context"
	"fmt"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/ipfs/go-ipfs-api/options"

	"github.com/ipfs/go-graphsync"
	gsync "github.com/ipfs/go-graphsync"
	gsmsg "github.com/ipfs/go-graphsync/message"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	ipldselector "github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

type ReceivedMessage struct {
	Message gsmsg.GraphSyncMessage
	Sender  peer.ID
}

// Receiver is an interface for receiving messages from the GraphSyncNetwork.
type Receiver struct {
	MessageReceived chan ReceivedMessage
}

func (r *Receiver) ReceiveMessage(
	ctx context.Context,
	sender peer.ID,
	incoming gsmsg.GraphSyncMessage) {

	select {
	case <-ctx.Done():
	case r.MessageReceived <- ReceivedMessage{incoming, sender}:
	}
}

func (r *Receiver) ReceiveError(_ peer.ID, err error) {
	fmt.Println("got receive err")
}

func (r *Receiver) Connected(p peer.ID) {
}

func (r *Receiver) Disconnected(p peer.ID) {
}

// VerifyHasErrors verifies that at least one error was sent over a channel
func VerifyHasErrors(ctx context.Context, errChan <-chan error) error {

	for {
		select {
		case e, ok := <-errChan:
			if ok {
				return nil
			} else {
				return e
			}
		case <-ctx.Done():
		}
	}
}

// VerifyHasErrors verifies that at least one error was sent over a channel
func PrintProgress(ctx context.Context, pgChan <-chan gsync.ResponseProgress) {
	errCount := 0
	for {
		select {
		case _, ok := <-pgChan:
			if ok {
				fmt.Println("ok")
			}
			errCount++
		case <-ctx.Done():
		}
	}

}

var SelectAll ipld.Node = func() ipld.Node {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	return ssb.ExploreRecursive(
		ipldselector.RecursionLimitDepth(100), // default max
		ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
	).Node()
}()

func FetchBlock(ctx context.Context, exchange graphsync.GraphExchange, ipfspeer *peer.AddrInfo, c ipld.Link) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resps, errs := exchange.Request(ctx, ipfspeer.ID, c, SelectAll)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-resps:
			if !ok {
				resps = nil
			}
		case err, ok := <-errs:
			if !ok {
				// done.
				return nil
			}
			if err != nil {
				return fmt.Errorf("got an unexpected error: %s", err)
			}
		}
	}
}
func PushBlock(ctx context.Context, ipfsurl string, data []byte) (string, error) {

	s := shell.NewShell(ipfsurl)
	return s.DagPutWithOpts(data, options.Dag.Pin("true"), options.Dag.StoreCodec("dag-json"))

}

//Push block with extension data
func PushBlockWithExtData(ctx context.Context, exchange graphsync.GraphExchange, ipfspeer *peer.AddrInfo, c ipld.Link, extensionData gsync.ExtensionData, selector ipld.Node) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resps, errs := exchange.Request(ctx, ipfspeer.ID, c, selector, extensionData)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-resps:
			if !ok {
				resps = nil
			}
		case err, ok := <-errs:
			if !ok {
				// done.
				return nil
			}
			if err != nil {
				return fmt.Errorf("got an unexpected error: %s", err)
			}
		}
	}
}
