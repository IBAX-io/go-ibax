package types

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/pkg/errors"
)

const (
	firstBlock   = 1
	minBlockSize = 9
)

var ErrBlockSize = errors.New("Bad block size")

//BlockData is a structure of the block's header
type BlockData struct {
	BlockID           int64
	Time              int64
	EcosystemID       int64
	KeyID             int64
	NodePosition      int64
	Sign              []byte
	Hash              []byte
	RollbacksHash     []byte
	Version           int
	PrivateBlockchain bool
}

func (b BlockData) String() string {
	return fmt.Sprintf("BlockID:%d, Time:%d, NodePosition %d", b.BlockID, b.Time, b.NodePosition)
}

func blockVer(cur, prev *BlockData) (ret string) {
	if cur.Version >= consts.BvRollbackHash {
		ret = fmt.Sprintf(",%x", prev.RollbacksHash)
	}
	return
}

func (b *BlockData) ForSha(prev *BlockData, mrklRoot []byte) string {
	return fmt.Sprintf("%d,%x,%s,%d,%d,%d,%d",
		b.BlockID, prev.Hash, mrklRoot, b.Time, b.EcosystemID, b.KeyID, b.NodePosition) +
		blockVer(b, prev)
}

// ForSign from 128 bytes to 512 bytes. Signature of TYPE, BLOCK_ID, PREV_BLOCK_HASH, TIME, WALLET_ID, state_id, MRKL_ROOT
func (b *BlockData) ForSign(prev *BlockData, mrklRoot []byte) string {
	return fmt.Sprintf("0,%v,%x,%v,%v,%v,%v,%s",
		b.BlockID, prev.Hash, b.Time, b.EcosystemID, b.KeyID, b.NodePosition, mrklRoot) +
		blockVer(b, prev)
}

// ParseBlockHeader is parses block header
func ParseBlockHeader(buf *bytes.Buffer, maxBlockSize int64) (header, prev BlockData, err error) {
	if buf.Len() < minBlockSize {
		err = ErrBlockSize
		return
	}

	header.Version = int(converter.BinToDec(buf.Next(2)))
	header.BlockID = converter.BinToDec(buf.Next(4))
	header.Time = converter.BinToDec(buf.Next(4))
	header.EcosystemID = converter.BinToDec(buf.Next(4))
	header.KeyID, err = converter.DecodeLenInt64Buf(buf)
	if err != nil {
		return
	}
	header.NodePosition = converter.BinToDec(buf.Next(1))

	// for version of block with included the rollback hash
	if header.Version >= consts.BvIncludeRollbackHash {
		prev.RollbacksHash, err = converter.DecodeBytesBuf(buf)
		if err != nil {
			return
		}
	}

	if header.BlockID == firstBlock {
		buf.Next(1)
		return
	}

	if int64(buf.Len()) > maxBlockSize {
		err = ErrBlockSize
		return
	}

	header.Sign, err = converter.DecodeBytesBuf(buf)
	if err != nil {
		return
	}

	return
}
