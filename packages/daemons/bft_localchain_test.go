/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

// The tests of this file run on a local chain of the BFT consensus, as dapps
// tools/localchain/f18.sh runs them, and skip without BFT_LOCALCHAIN_CONFIG, the config.toml of a
// node of the chain whose database and keys they take. They print their results as lines
// NAME VALUE for the script.

// localChain loads the node of BFT_LOCALCHAIN_CONFIG as the node does when it starts, without
// starting it
func localChain(t *testing.T) {
	cfg := os.Getenv("BFT_LOCALCHAIN_CONFIG")
	if cfg == "" {
		t.Skip("BFT_LOCALCHAIN_CONFIG is not set")
	}
	if err := conf.LoadConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := sqldb.GormInit(conf.Config.DB); err != nil {
		t.Fatal(err)
	}
	if err := syspar.ReadNodeKeys(); err != nil {
		t.Fatal(err)
	}
	log.SetLevel(log.FatalLevel)
	data, ok := block.GetDataFromFirstBlock()
	if !ok {
		t.Fatal("no first block")
	}
	syspar.SetFirstBlockData(data)
	syspar.SetRunModel(consts.HonorNodeMode)
	if err := syspar.SysUpdate(nil); err != nil {
		t.Fatal(err)
	}
	if err := syspar.SysTableColType(nil); err != nil {
		t.Fatal(err)
	}
	smart.InitVM()
	if err := smart.LoadContracts(); err != nil {
		t.Fatal(err)
	}
}

// nodeKeys is the node keys of the data directory of a node, the public key as the sets of the
// validators have it
func nodeKeys(t *testing.T, dir string) (private, public []byte) {
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		key, err := hex.DecodeString(string(bytes.TrimSpace(b)))
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	return read(consts.NodePrivateKeyFilename), crypto.CutPub(read(consts.NodePublicKeyFilename))
}

func envHeight(t *testing.T, name string) uint64 {
	h, err := strconv.ParseUint(os.Getenv(name), 10, 64)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return h
}

// TestLocalChainEvidence makes the evidence of two prevotes of the validator of the node keys of
// the data directory BFT_EVIDENCE_SIGNER at the height BFT_EVIDENCE_HEIGHT, round 0, for two
// different hashes, and prints it as EVIDENCE HEX; with BFT_EVIDENCE_SUBMIT=1 it sends the
// evidence transaction signed by the node of BFT_LOCALCHAIN_CONFIG, as a validator makes it, to
// the API of that node, which keeps its pool in its process, and prints SUBMITTED once a block
// has it
func TestLocalChainEvidence(t *testing.T) {
	localChain(t)
	priv, pub := nodeKeys(t, os.Getenv("BFT_EVIDENCE_SIGNER"))
	height := envHeight(t, "BFT_EVIDENCE_HEIGHT")
	set, err := validatorsAt(height)
	if err != nil {
		t.Fatal(err)
	}
	index, ok := set.Index(pub)
	if !ok {
		t.Fatalf("the signer is not in the set of height %d", height)
	}
	networkID := uint64(conf.Config.LocalConf.NetworkID)
	var a, b bft.Hash
	a[0], b[0] = 1, 2
	sigA, err := bft.Sign(priv, networkID, height, 0, bft.TypePrevote, a, bft.NoRound)
	if err != nil {
		t.Fatal(err)
	}
	sigB, err := bft.Sign(priv, networkID, height, 0, bft.TypePrevote, b, bft.NoRound)
	if err != nil {
		t.Fatal(err)
	}
	e, err := bft.NewEvidence(networkID, height, 0, index, bft.TypePrevote, a, b, bft.NoRound, bft.NoRound, sigA, sigB)
	if err != nil {
		t.Fatal(err)
	}
	body := e.Marshal()
	fmt.Printf("EVIDENCE %x\n", body)
	if os.Getenv("BFT_EVIDENCE_SUBMIT") == "1" {
		sendEvidenceTx(t, body)
		fmt.Println("SUBMITTED")
	}
}

// sendEvidenceTx sends the evidence transaction of the node of BFT_LOCALCHAIN_CONFIG to its API,
// logged in with its node key, and waits for a block to have the transaction
func sendEvidenceTx(t *testing.T, body []byte) {
	txData, err := evidenceTx(body)
	if err != nil {
		t.Fatal(err)
	}
	host := conf.Config.HTTP.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	connect := &api.Connect{
		Root:       fmt.Sprintf("http://%s/api/v2/", net.JoinHostPort(host, strconv.Itoa(conf.Config.HTTP.Port))),
		PrivateKey: syspar.GetNodePrivKey(),
		PublicKey:  hex.EncodeToString(crypto.NodeAccountKey(syspar.GetNodePubKey()).Bytes()),
	}
	if err := connect.Login(); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Hashes map[string]string `json:"hashes"`
	}
	if err := connect.SendMultipart("sendTx", map[string][]byte{"evidence": txData}, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Hashes) != 1 {
		t.Fatalf("the node took %d transactions, not 1", len(sent.Hashes))
	}
	for hash := range sent.Hashes {
		if blockID, err := connect.WaitTx(hash); blockID == 0 {
			t.Fatalf("the evidence transaction %s: %v", hash, err)
		}
	}
}

// storedHeight is the block and the certificate of a height as the node stores them
func storedHeight(t *testing.T, height int64) (raw, cert []byte) {
	b := &sqldb.BlockChain{}
	found, err := b.Get(height)
	if err != nil || !found {
		t.Fatalf("block %d: found %v, %v", height, found, err)
	}
	c := &sqldb.BftCertificate{}
	found, err = c.Get(nil, height)
	if err != nil || !found {
		t.Fatalf("the certificate of block %d: found %v, %v", height, found, err)
	}
	return b.Data, c.Data
}

// certifySynced checks a block and its certificate as the node checks those a peer sends it
// (commitSynced) before it executes the block
func certifySynced(height uint64, raw, data []byte) error {
	cert, err := syncedCertificate(height, data)
	if err != nil {
		return err
	}
	_, err = block.Certify(raw, cert)
	return err
}

// TestLocalChainCertificates checks the stored chain of the node as a node that synchronizes it
// checks it: every block from 2 up to the last one has a certificate of a quorum of the
// validators of its height that commits it. It prints CERTIFIED with the heights it checked and
// their least precommits. Then it alters the certificate or the block of the last height with
// transactions in the ways of the cases, each of which the checks must refuse, and prints REFUSED
// CASE for each.
func TestLocalChainCertificates(t *testing.T) {
	localChain(t)
	last, err := lastBlock()
	if err != nil {
		t.Fatal(err)
	}
	least, withTxs := 0, int64(0)
	for h := uint64(2); h <= last; h++ {
		raw, data := storedHeight(t, int64(h))
		if err := certifySynced(h, raw, data); err != nil {
			t.Fatalf("block %d: %v", h, err)
		}
		cert, _ := bft.UnmarshalCertificate(data)
		if least == 0 || len(cert.Precommits) < least {
			least = len(cert.Precommits)
		}
		if b := (&types.BlockData{}); proto.Unmarshal(raw, b) == nil && len(b.TxFullData) > 0 {
			withTxs = int64(h)
		}
	}
	fmt.Printf("CERTIFIED 2-%d least_precommits %d\n", last, least)
	if withTxs < 3 {
		t.Fatal("no block of transactions to alter above height 2")
	}

	raw, data := storedHeight(t, withTxs)
	prevRaw, _ := storedHeight(t, withTxs-1)
	set, err := validatorsAt(uint64(withTxs))
	if err != nil {
		t.Fatal(err)
	}
	certOf := func() *bft.Certificate {
		c, err := bft.UnmarshalCertificate(data)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if len(certOf().Precommits) < 2 {
		t.Fatal("a certificate of fewer than 2 precommits")
	}
	altered := func(f func(c *bft.Certificate)) []byte {
		c := certOf()
		f(c)
		return c.Marshal()
	}
	blockOf := func(f func(b *types.BlockData)) []byte {
		b := &types.BlockData{}
		if err := proto.Unmarshal(raw, b); err != nil {
			t.Fatal(err)
		}
		f(b)
		out, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	cases := []struct {
		name      string
		raw, data []byte
	}{
		{"signature", raw, altered(func(c *bft.Certificate) { c.Precommits[0].Signature[len(c.Precommits[0].Signature)-1] ^= 1 })},
		{"hash", raw, altered(func(c *bft.Certificate) { c.Hash[0] ^= 1 })},
		{"height", raw, altered(func(c *bft.Certificate) { c.Height++ })},
		{"round", raw, altered(func(c *bft.Certificate) { c.Round++ })},
		{"below_quorum", raw, altered(func(c *bft.Certificate) { c.Precommits = c.Precommits[:set.Quorum()-1] })},
		{"out_of_order", raw, altered(func(c *bft.Certificate) { c.Precommits[0], c.Precommits[1] = c.Precommits[1], c.Precommits[0] })},
		{"twice", raw, altered(func(c *bft.Certificate) { c.Precommits[1] = c.Precommits[0] })},
		{"no_validator", raw, altered(func(c *bft.Certificate) { c.Precommits[len(c.Precommits)-1].Validator = uint32(set.N()) })},
		{"another_block", prevRaw, data},
		{"block_hash", blockOf(func(b *types.BlockData) {
			b.Header.BlockHash = bytes.Clone(b.Header.BlockHash)
			b.Header.BlockHash[0] ^= 1
		}), data},
		{"block_content", blockOf(func(b *types.BlockData) {
			tx := bytes.Clone(b.TxFullData[0])
			tx[len(tx)-1] ^= 1
			b.TxFullData[0] = tx
		}), data},
	}
	for _, c := range cases {
		if err := certifySynced(uint64(withTxs), c.raw, c.data); err == nil {
			t.Errorf("%s: the altered height %d is taken", c.name, withTxs)
		} else {
			fmt.Printf("REFUSED %s %v\n", c.name, err)
		}
	}
}

// TestLocalChainFlood sends the consensus messages a node must drop or refuse to the node at the
// address BFT_FLOOD_ADDR, the TCP address of a validator: votes of a key of no validator, votes
// of the validator of the node keys of the data directory BFT_FLOOD_VALIDATOR out of the window
// of the heights, and frames that do not decode, at a rate under bft_peer_msg_rate, on a stream
// the node must keep; then three times bft_peer_msg_rate frames at once, past which the node
// must close the stream and refuse a new one. It prints SENT with the counts of each.
func TestLocalChainFlood(t *testing.T) {
	localChain(t)
	addr := os.Getenv("BFT_FLOOD_ADDR")
	vPriv, vPub := nodeKeys(t, os.Getenv("BFT_FLOOD_VALIDATOR"))
	last, err := lastBlock()
	if err != nil {
		t.Fatal(err)
	}
	height := last + 1
	set, err := validatorsAt(last)
	if err != nil {
		t.Fatal(err)
	}
	index, ok := set.Index(vPub)
	if !ok {
		t.Fatal("BFT_FLOOD_VALIDATOR is no validator")
	}
	outsider, _, err := crypto.GenNodeKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	networkID := uint64(conf.Config.LocalConf.NetworkID)
	vote := func(priv []byte, validator uint32, h uint64, i int) []byte {
		v := &bft.Vote{Type: bft.TypePrevote, Height: h, Round: 0, Validator: validator}
		v.Hash[0], v.Hash[1] = byte(i), byte(i>>8)
		if err := v.Sign(priv, networkID); err != nil {
			t.Fatal(err)
		}
		return bftnet.AppendFrame(nil, bftnet.FrameVote, v.Marshal())
	}
	open := func() net.Conn {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := (&network.RequestType{Type: network.RequestTypeBftMessages}).Write(conn); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	// closed reports whether the node closed the stream within a time: it writes nothing on it
	closed := func(conn net.Conn, within time.Duration) bool {
		conn.SetReadDeadline(time.Now().Add(within))
		_, err := conn.Read(make([]byte, 1))
		var ne net.Error
		return !(errors.As(err, &ne) && ne.Timeout())
	}
	rate := syspar.GetBftPeerMsgRate()
	const each = 50
	pace := time.Second / time.Duration(rate/5)

	conn := open()
	defer conn.Close()
	var frames [][]byte
	for i := range each {
		frames = append(frames, vote(outsider, uint32(i%set.N()), height, i))
	}
	for i := range each {
		// heights the chain never reaches, so that no vote of the validator conflicts with these
		frames = append(frames, vote(vPriv, index, height+1<<40+uint64(i), i))
	}
	for i := range each {
		junk := make([]byte, 40)
		junk[0] = byte(i)
		frames = append(frames, bftnet.AppendFrame(nil, bftnet.FrameVote, junk))
	}
	for _, f := range frames {
		if _, err := conn.Write(f); err != nil {
			t.Fatalf("the node closed the stream under its rate: %v", err)
		}
		time.Sleep(pace)
	}
	if closed(conn, 2*time.Second) {
		t.Fatal("the node closed the stream under its rate")
	}
	fmt.Printf("SENT outsider %d out_of_window %d undecodable %d\n", each, each, each)

	flood := make([]byte, 0, 3*rate*120)
	for i := range 3 * rate {
		flood = append(flood, vote(outsider, 0, height, each+i)...)
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	conn.Write(flood)
	if !closed(conn, 5*time.Second) {
		t.Fatalf("the node keeps a stream of %d frames at once, past its rate of %d", 3*rate, rate)
	}
	fmt.Printf("SENT flood %d rate %d\n", 3*rate, rate)

	again := open()
	defer again.Close()
	again.Write(vote(outsider, 0, height, 0))
	if !closed(again, 5*time.Second) {
		t.Fatal("the node takes a new stream of the address it refuses")
	}
	fmt.Println("REFUSED stream")
}
