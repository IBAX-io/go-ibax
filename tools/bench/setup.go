/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// benchTable and benchContract are the table and the contract of the write-heavy workload: a
// transaction inserts 5 rows and updates each
const (
	benchTable    = "bench_rows"
	benchContract = "BenchWrite"
	benchSource   = `contract BenchWrite {
    data {
        N int
    }
    action {
        var i id int
        while i < 5 {
            var row update map
            row["n"] = $N
            row["k"] = i
            id = DBInsert("@1bench_rows", row)
            update["v"] = $N + i
            DBUpdate("@1bench_rows", id, update)
            i = i + 1
        }
    }
}`
)

// setupConfig is what setup makes: the accounts, each with coins of ecosystem 1 on its account and
// in UTXO and a member of ecosystem 1, and the table and the contract of the write-heavy workload
type setupConfig struct {
	file     string
	algo     string
	accounts int
	fund     decimal.Decimal
	batch    int
	timeout  time.Duration
}

// setup registers the accounts with self-signed @1NewUser, sends each coins from the founder, moves
// half to its UTXO, names it a member with ProfileEdit, and makes the table and the contract of the
// write-heavy workload
func setup(c *chainClient, cfg setupConfig) error {
	if _, err := os.Stat(cfg.file); err == nil {
		log.Printf("%s exists: its accounts are set up", cfg.file)
		return setupContract(c, cfg)
	}
	accounts, err := genAccounts(cfg.algo, cfg.accounts)
	if err != nil {
		return err
	}
	if err := saveAccounts(cfg.file, accounts); err != nil {
		return err
	}
	log.Printf("%d accounts of %s in %s", len(accounts), cfg.algo, cfg.file)

	ids, err := c.contractIDs("@1NewUser", "@1TokensSend", "@1ProfileEdit")
	if err != nil {
		return err
	}
	each := func(step string, sign func(i int, a *account) (*signedTx, error)) error {
		txs := make([]*signedTx, 0, len(accounts))
		for i, a := range accounts {
			tx, err := sign(i, a)
			if err != nil {
				return err
			}
			txs = append(txs, tx)
		}
		return c.run(step, txs, cfg.batch, cfg.timeout)
	}

	if err := each("NewUser", func(_ int, a *account) (*signedTx, error) {
		return c.signer.contract(a, ids["@1NewUser"], map[string]any{"NewPubkey": fmt.Sprintf("%x", a.public.Bytes())})
	}); err != nil {
		return err
	}
	if err := each("TokensSend from the founder", func(_ int, a *account) (*signedTx, error) {
		return c.signer.contract(c.founder, ids["@1TokensSend"], map[string]any{"Recipient": a.address(), "Amount": cfg.fund.String(), "Comment": "bench " + a.address()})
	}); err != nil {
		return err
	}
	half := cfg.fund.Div(decimal.NewFromInt(2)).Floor().String()
	if c.honorOnly {
		log.Print("TransferSelf to UTXO: none, a chain of the honor nodes only has no UTXO")
	} else if err := each("TransferSelf to UTXO", func(_ int, a *account) (*signedTx, error) {
		return c.signer.transferSelf(a, half, "Account", "UTXO")
	}); err != nil {
		return err
	}
	if err := each("ProfileEdit", func(i int, a *account) (*signedTx, error) {
		return c.signer.contract(a, ids["@1ProfileEdit"], map[string]any{"Name": "bench" + strconv.FormatInt(a.keyID&0x7fffffffffff, 36)})
	}); err != nil {
		return err
	}

	return setupContract(c, cfg)
}

// setupContract makes the table and the contract of the write-heavy workload, those not there
func setupContract(c *chainClient, cfg setupConfig) error {
	if _, err := c.nodes[0].contractID("@1" + benchContract); err == nil {
		log.Printf("%s exists", benchContract)
		return nil
	}
	ids, err := c.contractIDs("@1NewTableJoint", "@1NewContract")
	if err != nil {
		return err
	}
	if err := c.nodes[0].get("table/"+benchTable, nil); err == nil {
		log.Printf("%s exists", benchTable)
	} else {
		table, err := c.signer.contract(c.founder, ids["@1NewTableJoint"], map[string]any{
			"ApplicationId": int64(1), "Name": benchTable,
			"ColumnsArr": []any{"n", "k", "v"}, "TypesArr": []any{"number", "number", "number"},
			"InsertPerm": "true", "UpdatePerm": "true", "ReadPerm": "true", "NewColumnPerm": "true",
		})
		if err != nil {
			return err
		}
		if err := c.run("NewTableJoint "+benchTable, []*signedTx{table}, 1, cfg.timeout); err != nil {
			return err
		}
	}
	contract, err := c.signer.contract(c.founder, ids["@1NewContract"], map[string]any{
		"ApplicationId": int64(1), "Value": benchSource, "Conditions": "true",
	})
	if err != nil {
		return err
	}
	return c.run("NewContract "+benchContract, []*signedTx{contract}, 1, cfg.timeout)
}

// chainClient is the bench's hold of the network: its nodes, logged in with the founder key, and
// the signer of its transactions
type chainClient struct {
	nodes   []*node
	founder *account
	signer  *signer
	info    *chainInfo
	// honorOnly is a chain of the honor nodes only (honor_nodes_only, every chain of the BFT
	// consensus): it takes no TransferSelf nor UTXO transaction
	honorOnly bool
}

func connect(urls []string, keyFile, keyAlgo string) (*chainClient, error) {
	c := &chainClient{}
	for _, u := range urls {
		c.nodes = append(c.nodes, newNode(u))
	}
	// The address of a key is of the network's hash, and the founder key is of the node keys'
	// algorithm unless told otherwise: the first node tells both
	var uid struct {
		Cryptoer string `json:"cryptoer"`
		Hasher   string `json:"hasher"`
	}
	if err := c.nodes[0].get("getuid", &uid); err != nil {
		return nil, err
	}
	if err := useChain(&chainInfo{Cryptoer: uid.Cryptoer, Hasher: uid.Hasher}); err != nil {
		return nil, err
	}
	if keyAlgo == "" {
		keyAlgo = uid.Cryptoer
	}
	founder, err := readKey(keyFile, keyAlgo)
	if err != nil {
		return nil, err
	}
	c.founder = founder
	for _, n := range c.nodes {
		info, err := n.login(founder)
		if err != nil {
			return nil, fmt.Errorf("login to %s: %w", n.api, err)
		}
		if c.info != nil && *info != *c.info {
			return nil, fmt.Errorf("%s is of another network: %+v, not %+v", n.api, *info, *c.info)
		}
		c.info = info
	}
	c.signer = newSigner(c.info)
	var params struct {
		List []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"list"`
	}
	if err := c.nodes[0].get("systemparams?names=honor_nodes_only", &params); err != nil {
		var apiErr *apiError
		if !errors.As(err, &apiErr) {
			return nil, err
		}
	}
	for _, p := range params.List {
		c.honorOnly = p.Name == "honor_nodes_only" && (p.Value == "1" || p.Value == "true")
	}
	return c, nil
}

func (c *chainClient) contractIDs(names ...string) (map[string]int, error) {
	out := map[string]int{}
	for _, name := range names {
		id, err := c.nodes[0].contractID(name)
		if err != nil {
			return nil, fmt.Errorf("contract %s: %w", name, err)
		}
		out[name] = id
	}
	return out, nil
}

// run sends the transactions of a step of setup, batch at a time to the nodes in turn, and waits
// until each succeeded in a block; it fails on the first that failed
func (c *chainClient) run(step string, txs []*signedTx, batch int, timeout time.Duration) error {
	start := time.Now()
	for i := 0; i < len(txs); i += batch {
		if err := c.nodes[(i/batch)%len(c.nodes)].send(txs[i:min(i+batch, len(txs))]); err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
	}
	hashes := make([]string, len(txs))
	for i, tx := range txs {
		hashes[i] = tx.hash
	}
	failed, pending, err := c.await(hashes, timeout)
	if err != nil {
		return fmt.Errorf("%s: %w", step, err)
	}
	for h, why := range failed {
		return fmt.Errorf("%s: %d of %d failed, %s: %s", step, len(failed), len(txs), h, why)
	}
	if len(pending) > 0 {
		return fmt.Errorf("%s: %d of %d in no block after %s", step, len(pending), len(txs), timeout)
	}
	log.Printf("%s: %d in blocks in %.1fs", step, len(txs), time.Since(start).Seconds())
	return nil
}

// await asks the first node the status of the transactions until each succeeded in a block or
// failed, or the time runs out: it tells those that failed and why, and those still pending
func (c *chainClient) await(hashes []string, timeout time.Duration) (failed map[string]string, pending []string, err error) {
	failed = map[string]string{}
	pending = hashes
	deadline := time.Now().Add(timeout)
	for len(pending) > 0 && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		var still []string
		for i := 0; i < len(pending); i += 1000 {
			part := pending[i:min(i+1000, len(pending))]
			statuses, err := c.nodes[0].statuses(part)
			if err != nil {
				return nil, nil, err
			}
			for _, h := range part {
				s := statuses[h]
				switch {
				case s == nil || !s.done():
					still = append(still, h)
				case s.failure() != "":
					failed[h] = s.failure()
				}
			}
		}
		pending = still
	}
	return failed, pending, nil
}
