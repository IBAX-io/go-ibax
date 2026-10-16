package daemons

import (
	"bytes"
	"context"
	"io/fs"
	"os"

	"github.com/pkg/errors"

	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/rules"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

func InitialLoad(logger *log.Entry) error {

	// check for initial load
	toLoad, err := needLoad(logger)
	if err != nil {
		return err
	}

	if toLoad {
		logger.Debug("start first block loading")

		if err := firstLoad(logger); err != nil {
			logger.WithError(err).Error("cant load first block form file or host")
			return err
		}

		if err := sqldb.UpdateSchema(); err != nil {
			return err
		}
	}

	return nil
}

// loadFirstBlock inserts block 1: the node's own 1block file when it has one, else block 1 as the
// boot node with the highest block holds it. Each node of a new network holds the same 1block, so
// nodes that boot from one another can all start cold
func loadFirstBlock(logger *log.Entry) error {
	newBlock, err := readFirstBlock(conf.Config.DirPathConf.FirstBlockPath, len(conf.Config.BootNodes.NodesAddr) > 0,
		func() ([]byte, error) { return fetchFirstBlock(logger) })
	if err != nil {
		return err
	}
	if err = checkFirstBlock(newBlock); err != nil {
		return err
	}
	if err = block.InsertBlockWOForksNew(newBlock, nil, false, true); err != nil {
		logger.WithFields(log.Fields{"type": consts.ParserError, "error": err}).Error("inserting new block")
		return err
	}

	return nil
}

// readFirstBlock reads block 1 from the file at path; only a node without that file and with boot
// nodes fetches it
func readFirstBlock(path string, hasBootNodes bool, fetch func() ([]byte, error)) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, fs.ErrNotExist) || !hasBootNodes {
		return nil, errors.Wrap(err, "reading first block from file path")
	}
	return fetch()
}

// checkFirstBlock refuses a block 1 other than the first block of a network whose first block is
// known, such as a 1block file generated for another network
func checkFirstBlock(data []byte) error {
	want, ok := rules.GenesisHash()
	if !ok {
		return nil
	}
	b, err := block.UnmarshallBlock(bytes.NewBuffer(data), true)
	if err != nil {
		return errors.Wrap(err, "unmarshalling first block")
	}
	if !bytes.Equal(b.Header.BlockHash, want) {
		return errors.Errorf("first block hash %x is not the network's %x", b.Header.BlockHash, want)
	}
	return nil
}

// fetchFirstBlock fetches block 1 from the boot node with the highest block
func fetchFirstBlock(logger *log.Entry) ([]byte, error) {
	ctxDone, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, _, err := getHostWithMaxID(ctxDone, logger)
	if err != nil {
		return nil, errors.Wrap(err, "reading host")
	}
	rawBlocksChan, err := tcpclient.GetBlocksBodies(ctxDone, host, 1, true)
	if err != nil {
		return nil, err
	}
	var data []byte
	for rawBlock := range rawBlocksChan {
		data = rawBlock
	}
	return data, nil
}

func firstLoad(logger *log.Entry) error {
	DBLock()
	defer DBUnlock()

	return loadFirstBlock(logger)
}

func needLoad(logger *log.Entry) (bool, error) {
	infoBlock := &sqldb.InfoBlock{}
	_, err := infoBlock.Get()
	if err != nil {
		logger.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("getting info block")
		return false, err
	}
	// we have empty blockchain, we need to load blockchain from file or other source
	if infoBlock.BlockID == 0 {
		logger.Debug("blockchain should be loaded")
		return true, nil
	}
	return false, nil
}
