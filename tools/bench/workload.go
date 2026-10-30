/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"fmt"
	"log"
	"strconv"
)

// workload makes the transaction seq of a run, of the accounts in turn; no two of a run are alike
type workload struct {
	name string
	make func(seq int) (*signedTx, error)
}

// workloads are the workloads of the perf plan §4.2:
//
//	self      TransferSelf, UTXO to account and back in turn
//	utxo      UTXO transfers from each account to its peer
//	transfer  @1TokensSend of 1 from each account to its peer: the transfer of a chain of the honor
//	          nodes only, which takes no TransferSelf nor UTXO
//	contract  the contract mix: ProfileEdit, NotificationsSend and TokensSend, 4:3:3
//	write     BenchWrite: 5 DBInsert and 5 DBUpdate a transaction
func workloads(c *chainClient, accounts []*account, names []string, run int) ([]workload, error) {
	n := len(accounts)
	// Each run marks its transactions, which stay unlike those of the runs before it
	mark := func(seq int) string { return strconv.Itoa(run) + "-" + strconv.Itoa(seq) }
	// The peer of the account of a transaction is half the accounts away: the transactions that
	// follow one another touch other accounts, as the transfers of different users do, rather
	// than each spending what the one before it paid in
	peer := func(seq int) *account { return accounts[(seq+n/2)%n] }
	var out []workload
	for _, name := range names {
		if c.honorOnly && (name == "self" || name == "utxo") {
			log.Printf("%s: skipped, a chain of the honor nodes only takes no TransferSelf nor UTXO", name)
			continue
		}
		switch name {
		case "transfer":
			id, err := c.nodes[0].contractID("@1TokensSend")
			if err != nil {
				return nil, err
			}
			out = append(out, workload{name, func(seq int) (*signedTx, error) {
				return c.signer.contract(accounts[seq%n], id, map[string]any{"Recipient": peer(seq).address(), "Amount": "1", "Comment": mark(seq)})
			}})
		case "self":
			out = append(out, workload{name, func(seq int) (*signedTx, error) {
				a := accounts[seq%n]
				value := strconv.Itoa(1000 + run*1_000_000 + seq)
				if (seq/n)%2 == 0 {
					return c.signer.transferSelf(a, value, "UTXO", "Account")
				}
				return c.signer.transferSelf(a, value, "Account", "UTXO")
			}})
		case "utxo":
			out = append(out, workload{name, func(seq int) (*signedTx, error) {
				return c.signer.utxo(accounts[seq%n], peer(seq).keyID, "1", mark(seq))
			}})
		case "contract":
			ids, err := c.contractIDs("@1ProfileEdit", "@1NotificationsSend", "@1TokensSend")
			if err != nil {
				return nil, err
			}
			out = append(out, workload{name, func(seq int) (*signedTx, error) {
				a, next := accounts[seq%n], peer(seq)
				switch seq % 10 {
				case 0, 1, 2, 3:
					return c.signer.contract(a, ids["@1ProfileEdit"], map[string]any{"Name": "bench", "Info": "bench " + mark(seq)})
				case 4, 5, 6:
					return c.signer.contract(a, ids["@1NotificationsSend"], map[string]any{"Account": next.address(), "Header": "bench", "Body": mark(seq)})
				default:
					return c.signer.contract(a, ids["@1TokensSend"], map[string]any{"Recipient": next.address(), "Amount": "1", "Comment": mark(seq)})
				}
			}})
		case "write":
			id, err := c.nodes[0].contractID("@1" + benchContract)
			if err != nil {
				return nil, fmt.Errorf("contract %s, which setup makes: %w", benchContract, err)
			}
			out = append(out, workload{name, func(seq int) (*signedTx, error) {
				return c.signer.contract(accounts[seq%n], id, map[string]any{"N": int64(run)*1_000_000 + int64(seq)})
			}})
		default:
			return nil, fmt.Errorf("no workload %q: self, utxo, transfer, contract or write", name)
		}
	}
	return out, nil
}
