/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"

	log "github.com/sirupsen/logrus"
)

var ErrorEmptyBlockBody = errors.New("block is empty")

// ErrNotKept is the answer of a node that no longer keeps the blocks asked: another node, an
// archive one, keeps them
var ErrNotKept = errors.New("the node does not keep the blocks")
var ErrorWrongSizeBytes = errors.New("wrong size bytes")

const hasVal = "has value"
const hasntVal = "has not value"

const sizeBytesLength = 4

// GetBlocksBodies send GetBodiesRequest returns channel of binary blocks data
func GetBlocksBodies(ctx context.Context, host string, blockID int64, reverseOrder bool) (<-chan []byte, error) {
	blocks, _, err := GetBlocksBatch(ctx, host, blockID, reverseOrder)
	return blocks, err
}

// GetBlocksBatch asks host for the blocks after the block blockID, or up to it in reverse order,
// and returns the channel of their binary data with how many blocks the host sends
func GetBlocksBatch(ctx context.Context, host string, blockID int64, reverseOrder bool) (<-chan []byte, int64, error) {
	conn, err := newConnection(host)
	if err != nil {
		return nil, 0, err
	}
	fail := func(err error) (<-chan []byte, int64, error) {
		conn.Close()
		return nil, 0, err
	}

	// send the type of data
	rt := &network.RequestType{Type: network.RequestTypeBlockCollection}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("writing data type block body to connection")
		return fail(err)
	}

	req := &network.GetBodiesRequest{
		BlockID:      uint32(blockID),
		ReverseOrder: reverseOrder,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on sending blocks bodies request")
		return fail(err)
	}

	blocksCount, err := network.ReadInt(conn)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on getting blocks count")
		return fail(err)
	}

	if blocksCount == network.BlocksNotKept {
		return fail(fmt.Errorf("%w: %s, block %d", ErrNotKept, host, blockID))
	}
	if blocksCount == 0 {
		return fail(fmt.Errorf("host: %s does'nt contains blocks", host))
	}
	if blocksCount < 0 || blocksCount > int64(network.BlocksPerRequest) {
		return fail(fmt.Errorf("host: %s sends %d blocks", host, blocksCount))
	}

	blocksChan, errChan := GetBlockBodiesChan(ctx, conn, blocksCount)
	go func() {
		for err := range errChan {
			if err != nil {
				log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on reading block bodies")
			}
		}
	}()

	return blocksChan, blocksCount, nil
}

func GetBlockBodiesChan(ctx context.Context, src io.ReadCloser, blocksCount int64) (<-chan []byte, <-chan error) {
	rawBlocksCh := make(chan []byte, blocksCount)
	errChan := make(chan error, 1)

	sizeBuf := make([]byte, sizeBytesLength)
	var bodyBuf []byte

	afterBodyProcessed := func(done <-chan struct{}) {
		<-done
		BytesPool.Put(bodyBuf)
	}

	// a context done drops the blocks: the reading stops at once
	stop := context.AfterFunc(ctx, func() { src.Close() })
	go func() {
		defer func() {
			stop()
			close(rawBlocksCh)
			close(errChan)
			src.Close()
			if bodyBuf != nil {
				go afterBodyProcessed(ctx.Done())
			}
		}()

		dataSize, err := network.ReadInt(src)
		if err != nil {
			errChan <- err
			return
		}
		if dataSize < 0 || dataSize > network.MaxBlocksBytes {
			errChan <- fmt.Errorf("%w: blocks of %d bytes", ErrorWrongSizeBytes, dataSize)
			return
		}

		bodyBuf = BytesPool.Get(dataSize)
		var bodyStartIndx int64

		for i := 0; i < int(blocksCount); i++ {

			_, err := io.ReadFull(src, sizeBuf)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading size of block data")
				errChan <- err
				return
			}

			size, intErr := binary.Uvarint(sizeBuf)
			if intErr <= 0 || size > uint64(dataSize-bodyStartIndx) {
				log.WithFields(log.Fields{"type": consts.ConversionError, "error": ErrorWrongSizeBytes}).Error("on convert size body")
				errChan <- ErrorWrongSizeBytes
				return
			}

			bodyEndIndx := bodyStartIndx + int64(size)
			body := bodyBuf[bodyStartIndx:bodyEndIndx]
			if readed, err := io.ReadFull(src, body); err != nil {
				log.WithFields(log.Fields{"type": consts.IOError, "size": size, "readed": readed, "error": err}).Error("on reading block body")
				errChan <- err
				return
			}

			bodyStartIndx = bodyEndIndx
			rawBlocksCh <- body
			errChan <- nil
		}
	}()

	return rawBlocksCh, errChan
}
