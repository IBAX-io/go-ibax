/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"context"
	"io"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

func HostWithMaxBlock(ctx context.Context, hosts []string) (bestHost string, maxBlockID int64, err error) {
	if len(hosts) == 0 {
		return "", -1, nil
	}

	return hostWithMaxBlock(ctx, hosts)
}

func GetMaxBlockID(host string) (blockID int64, err error) {
	return getMaxBlock(host)
}

func getMaxBlock(host string) (blockID int64, err error) {
	con, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.ConnectionError, "host": host}).Debug("error connecting to host")
		return -1, err
	}
	defer con.Close()

	// send max block request
	rt := &network.RequestType{
		Type: network.RequestTypeMaxBlock,
	}

	if err := rt.Write(con); err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.ConnectionError, "host": host}).Error("on sending Max block request type")
		return -1, err
	}

	// response
	resp := network.MaxBlockResponse{}
	err = resp.Read(con)
	if err == io.EOF {
	} else if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.ConnectionError, "host": host}).Error("reading max block id from host")
		return -1, err
	}

	return resp.BlockID, nil
}

func hostWithMaxBlock(ctx context.Context, hosts []string) (bestHost string, maxBlockID int64, err error) {
	maxBlockID = -1

	type blockAndHost struct {
		host    string
		blockID int64
		err     error
	}

	resultChan := make(chan blockAndHost, len(hosts))

	/* rand.Shuffle(len(hosts), func(i, j int) { hosts[i], hosts[j] = hosts[j], hosts[i] })
	this implementation available only in Golang 1.10
	*/
	utils.ShuffleSlice(hosts)

	var wg sync.WaitGroup
	for _, h := range hosts {
		if ctx.Err() != nil {
			log.WithFields(log.Fields{"error": ctx.Err(), "type": consts.ContextError}).Error("context error")
			return "", maxBlockID, ctx.Err()
		}

		wg.Add(1)

		go func(host string) {
			blockID, err := getMaxBlock(host)
			defer wg.Done()

			resultChan <- blockAndHost{
				host:    host,
				blockID: blockID,
				err:     err,
			}
		}(h)
	}
	wg.Wait()

	var errCount int
	for i := 0; i < len(hosts); i++ {
		bl := <-resultChan

		if bl.err != nil {
			errCount++
			continue
		}

		// If blockID is maximal then the current host is the best
		if bl.blockID > maxBlockID {
			maxBlockID = bl.blockID
			bestHost = bl.host
		}
	}

	if errCount == len(hosts) {
		return "", 0, ErrNodesUnavailable
	}

	return bestHost, maxBlockID, nil
}
