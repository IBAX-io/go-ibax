/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// The DeFi workloads run iSwap (IBAX-io/iswap-contracts), the exchange of Uniswap V2 on IBAX, in
// an ecosystem of its own, over token ecosystems of the founder: iswap installs it and keeps what
// it made in a file, which the workloads read.
const (
	// iswapSupply is the supply of each token, iswapFund what each account gets of each token, and
	// iswapLiquidity what the founder adds of each token to each pair, in the smallest units
	iswapSupply    = "1000000000000000000000000000"
	iswapFund      = "100000000000000000"
	iswapLiquidity = "1000000000000000000000"
	// iswapAmountIn is the amount in of a swap, iswapAddAmount of each token an add of liquidity
	// adds: small against the liquidity, a run moves the prices little
	iswapAmountIn  = 1_000_000_000_000
	iswapAddAmount = 1_000_000_000_000
	// iswapDeadline is how long after it is signed a transaction of the routers may run
	iswapDeadline = 7 * 24 * time.Hour
)

// deadline is the Deadline of a transaction of the routers signed now
func deadline() int64 { return time.Now().Add(iswapDeadline).Unix() }

// iswapConfig is what iswap installs: the bundle of iSwap, the token ecosystems, and the accounts
// of the bench that trade
type iswapConfig struct {
	bundle   string
	state    string
	tokens   int
	accounts []*account
	batch    int
	inFlight int
	timeout  time.Duration
}

// iswapState is what iswap made: the ecosystem of iSwap, the token ecosystems, their pairs, each
// of the lower ecosystem first, and the accounts that trade, the first of the accounts file; step
// is the steps done, so that iswap goes on where it stopped
type iswapState struct {
	Ecosystem int64      `json:"ecosystem"`
	Tokens    []int64    `json:"tokens"`
	Pairs     [][2]int64 `json:"pairs"`
	Accounts  int        `json:"accounts"`
	Step      int        `json:"step"`
}

func loadIswap(file string) (*iswapState, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var s iswapState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &s, nil
}

func (s *iswapState) save(file string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, b, 0o600)
}

// iswapStep is a step of iswap
type iswapStep struct {
	name string
	run  func(c *chainClient, cfg iswapConfig, s *iswapState) error
}

// iswapSteps are the steps of iswap in their order
var iswapSteps = []iswapStep{
	{"ecosystems", iswapEcosystems},
	{"tokens", iswapTokens},
	{"import", iswapImport},
	{"pairs", iswapPairs},
	{"members", iswapMembers},
	{"funds", iswapFunds},
	{"debit", iswapDebit},
	{"liquidity", iswapAddLiquidity},
	{"check", iswapCheck},
}

// iswap installs iSwap and readies the accounts to trade, the steps the state file has not done
func iswap(c *chainClient, cfg iswapConfig) error {
	s, err := loadIswap(cfg.state)
	if errors.Is(err, os.ErrNotExist) {
		s, err = &iswapState{Accounts: len(cfg.accounts)}, nil
	}
	if err != nil {
		return err
	}
	if s.Accounts != len(cfg.accounts) {
		return fmt.Errorf("%s: iSwap is set up for %d accounts, not %d", cfg.state, s.Accounts, len(cfg.accounts))
	}
	for s.Step < len(iswapSteps) {
		step := iswapSteps[s.Step]
		log.Printf("iswap: %s", step.name)
		if err := step.run(c, cfg, s); err != nil {
			return fmt.Errorf("iswap %s: %w", step.name, err)
		}
		s.Step++
		if err := s.save(cfg.state); err != nil {
			return err
		}
	}
	log.Printf("iswap: ecosystem %d, tokens %v, %d pairs, %d accounts", s.Ecosystem, s.Tokens, len(s.Pairs), s.Accounts)
	return nil
}

// one runs a transaction of the founder and tells the result of its contract
func (c *chainClient) one(step string, tx *signedTx, err error, timeout time.Duration) (string, error) {
	if err != nil {
		return "", err
	}
	results, err := c.runFor(step, []*signedTx{tx}, 1, timeout)
	return results[tx.hash], err
}

// fail runs a transaction that must fail, and tells an error if it did not or failed for another
// reason than want
func (c *chainClient) fail(step string, tx *signedTx, err error, want string, timeout time.Duration) error {
	if err != nil {
		return err
	}
	if err := c.nodes[0].send([]*signedTx{tx}); err != nil {
		if strings.Contains(err.Error(), want) {
			log.Printf("%s: refused, %s", step, want)
			return nil
		}
		return fmt.Errorf("%s: %w", step, err)
	}
	_, failed, pending, err := c.await([]string{tx.hash}, timeout)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%s: in no block after %s", step, timeout)
	}
	why, ok := failed[tx.hash]
	if !ok {
		return fmt.Errorf("%s: succeeded, it must fail with %q", step, want)
	}
	if !strings.Contains(why, want) {
		return fmt.Errorf("%s: failed with %q, not %q", step, why, want)
	}
	log.Printf("%s: failed, %s", step, want)
	return nil
}

// iswapEcosystems makes the ecosystem of iSwap and the token ecosystems
func iswapEcosystems(c *chainClient, cfg iswapConfig, s *iswapState) error {
	id, err := c.nodes[0].contractID("@1NewEcosystem")
	if err != nil {
		return err
	}
	names := []string{"iswap"}
	for i := range cfg.tokens {
		names = append(names, "iswap-token-"+strconv.Itoa(i))
	}
	var txs []*signedTx
	for _, name := range names {
		tx, err := c.signer.contract(c.founder, id, map[string]any{"Name": name})
		if err != nil {
			return err
		}
		txs = append(txs, tx)
	}
	results, err := c.runFor("NewEcosystem", txs, cfg.batch, cfg.timeout)
	if err != nil {
		return err
	}
	for i, tx := range txs {
		eco, err := strconv.ParseInt(results[tx.hash], 10, 64)
		if err != nil || eco <= 1 {
			return fmt.Errorf("NewEcosystem %s: result %q", names[i], results[tx.hash])
		}
		if i == 0 {
			s.Ecosystem = eco
		} else {
			s.Tokens = append(s.Tokens, eco)
		}
	}
	for i := range s.Tokens {
		for j := i + 1; j < len(s.Tokens); j++ {
			a, b := s.Tokens[i], s.Tokens[j]
			s.Pairs = append(s.Pairs, [2]int64{min(a, b), max(a, b)})
		}
	}
	return nil
}

// iswapTokens opens the ecosystems of iSwap and of the tokens to new members, and issues the tokens
func iswapTokens(c *chainClient, cfg iswapConfig, s *iswapState) error {
	ids, err := c.contractIDs("@1EditParameter", "@1NewParameter", "@1NewToken")
	if err != nil {
		return err
	}
	var open, issue []*signedTx
	for _, eco := range append([]int64{s.Ecosystem}, s.Tokens...) {
		// a new ecosystem has no free_membership: closed
		var p struct {
			ID string `json:"id"`
		}
		var tx *signedTx
		err := c.nodes[0].get("ecosystemparam/free_membership?ecosystem="+strconv.FormatInt(eco, 10), &p)
		var apiErr *apiError
		switch {
		case errors.As(err, &apiErr) && apiErr.Code == "E_PARAMNOTFOUND":
			tx, err = c.signer.contractIn(c.founder, eco, ids["@1NewParameter"], map[string]any{"Name": "free_membership", "Value": "1", "Conditions": `ContractConditions("@1DeveloperCondition")`})
		case err != nil:
			return fmt.Errorf("free_membership of ecosystem %d: %w", eco, err)
		default:
			pid, perr := strconv.ParseInt(p.ID, 10, 64)
			if perr != nil {
				return fmt.Errorf("free_membership of ecosystem %d: id %q", eco, p.ID)
			}
			tx, err = c.signer.contractIn(c.founder, eco, ids["@1EditParameter"], map[string]any{"Id": pid, "Value": "1"})
		}
		if err != nil {
			return err
		}
		open = append(open, tx)
	}
	for i, eco := range s.Tokens {
		symbol := "IST" + strconv.Itoa(i)
		tx, err := c.signer.contractIn(c.founder, eco, ids["@1NewToken"], map[string]any{"Symbol": symbol, "Name": "iSwap test " + symbol, "Amount": iswapSupply, "Digits": "6"})
		if err != nil {
			return err
		}
		issue = append(issue, tx)
	}
	if err := c.run("free_membership 1", open, cfg.batch, cfg.timeout); err != nil {
		return err
	}
	return c.run("NewToken", issue, cfg.batch, cfg.timeout)
}

// iswapImport imports the bundle of iSwap into its ecosystem, as the import page does:
// @1ImportUpload of the bundle, then @1Import of its pieces
func iswapImport(c *chainClient, cfg iswapConfig, s *iswapState) error {
	body, err := os.ReadFile(cfg.bundle)
	if err != nil {
		return err
	}
	var b struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return fmt.Errorf("%s: %w", cfg.bundle, err)
	}
	ids, err := c.contractIDs("@1ImportUpload", "@1Import")
	if err != nil {
		return err
	}
	file := map[string]any{"Name": filepath.Base(cfg.bundle), "MimeType": "application/json", "Body": body}
	tx, err := c.signer.contractIn(c.founder, s.Ecosystem, ids["@1ImportUpload"], map[string]any{"Data": file})
	if _, err := c.one("ImportUpload", tx, err, cfg.timeout); err != nil {
		return err
	}
	pieces, err := importPieces(b.Data)
	if err != nil {
		return err
	}
	for i, piece := range pieces {
		tx, err := c.signer.contractIn(c.founder, s.Ecosystem, ids["@1Import"], map[string]any{"Data": piece})
		if _, err := c.one(fmt.Sprintf("Import %d/%d", i+1, len(pieces)), tx, err, cfg.timeout); err != nil {
			return err
		}
	}
	return nil
}

// importPieces are the pieces of @1Import that @1ImportUpload cuts a bundle into: the items other
// than contracts and deletes by 10 positions of the bundle, then every contract, then every delete
func importPieces(items []map[string]any) ([]string, error) {
	const size = 10
	var out []string
	add := func(piece []map[string]any) error {
		if len(piece) == 0 {
			return nil
		}
		data, err := json.Marshal(piece)
		out = append(out, string(data))
		return err
	}
	var contracts, deletes []map[string]any
	for i := 0; i < len(items); i += size {
		var piece []map[string]any
		for _, item := range items[i:min(i+size, len(items))] {
			switch {
			case item["Delete"] == true:
				deletes = append(deletes, item)
			case item["Type"] == "contracts":
				contracts = append(contracts, item)
			default:
				piece = append(piece, item)
			}
		}
		if err := add(piece); err != nil {
			return nil, err
		}
	}
	if err := add(contracts); err != nil {
		return nil, err
	}
	return out, add(deletes)
}

// iswapID is the id of a contract of iSwap
func (c *chainClient) iswapID(s *iswapState, name string) (int, error) {
	return c.nodes[0].contractID("@" + strconv.FormatInt(s.Ecosystem, 10) + name)
}

// iswapPairs initializes iSwap and creates the pairs
func iswapPairs(c *chainClient, cfg iswapConfig, s *iswapState) error {
	initID, err := c.iswapID(s, "InitParam")
	if err != nil {
		return err
	}
	createID, err := c.iswapID(s, "CreatePair")
	if err != nil {
		return err
	}
	tx, err := c.signer.contractIn(c.founder, s.Ecosystem, initID, map[string]any{})
	if _, err := c.one("InitParam", tx, err, cfg.timeout); err != nil {
		return err
	}
	var txs []*signedTx
	for _, p := range s.Pairs {
		tx, err := c.signer.contractIn(c.founder, s.Ecosystem, createID, map[string]any{"Ecosystem1": p[0], "Ecosystem2": p[1], "Topic": fmt.Sprintf("%d-%d", p[0], p[1])})
		if err != nil {
			return err
		}
		txs = append(txs, tx)
	}
	return c.run("CreatePair", txs, cfg.batch, cfg.timeout)
}

// each signs a transaction of each account and runs them
func (c *chainClient) each(step string, accounts []*account, cfg iswapConfig, sign func(a *account) (*signedTx, error)) error {
	txs := make([]*signedTx, 0, len(accounts))
	for _, a := range accounts {
		tx, err := sign(a)
		if err != nil {
			return err
		}
		txs = append(txs, tx)
	}
	return c.run(step, txs, cfg.batch, cfg.timeout)
}

// iswapMembers makes the accounts members of the ecosystem of iSwap, whose transactions they sign,
// and of the token ecosystems, whose tokens they trade
func iswapMembers(c *chainClient, cfg iswapConfig, s *iswapState) error {
	id, err := c.nodes[0].contractID("@1NewUser")
	if err != nil {
		return err
	}
	for _, eco := range append([]int64{s.Ecosystem}, s.Tokens...) {
		if err := c.each(fmt.Sprintf("NewUser of ecosystem %d", eco), cfg.accounts, cfg, func(a *account) (*signedTx, error) {
			return c.signer.contract(a, id, map[string]any{"NewPubkey": fmt.Sprintf("%x", a.public.Bytes()), "Ecosystem": eco})
		}); err != nil {
			return err
		}
	}
	return nil
}

// iswapFunds sends each account tokens of each token ecosystem from the founder, inFlight at a time
func iswapFunds(c *chainClient, cfg iswapConfig, s *iswapState) error {
	id, err := c.nodes[0].contractID("@1TokensSend")
	if err != nil {
		return err
	}
	var txs []*signedTx
	for _, eco := range s.Tokens {
		for _, a := range cfg.accounts {
			tx, err := c.signer.contract(c.founder, id, map[string]any{"Recipient": a.address(), "Amount": iswapFund, "Comment": "iswap", "Ecosystem": eco})
			if err != nil {
				return err
			}
			txs = append(txs, tx)
		}
	}
	part := max(cfg.inFlight, 1)
	for i := 0; i < len(txs); i += part {
		if err := c.run(fmt.Sprintf("TokensSend of the tokens %d-%d", i+1, min(i+part, len(txs))), txs[i:min(i+part, len(txs))], cfg.batch, cfg.timeout); err != nil {
			return err
		}
	}
	return nil
}

// iswapRouters are the contracts of iSwap that debit the signer of the transaction, which the
// parameter signer_debit_contracts of ecosystem 1 names
var iswapRouters = []string{
	"SwapExactTokensForTokens", "SwapTokensForExactTokens", "MultiSwapExactTokensForTokens", "MultiSwapTokensForExactTokens",
	"AddLiquidity", "NewPairApply", "VotePairApply", "VotePairAuction", "DividendNFTMint",
}

// iswapDebit lets the routers of iSwap debit the signer: it sets the parameter
// signer_debit_contracts of ecosystem 1 to their full names, with those of other contracts it
// names kept
func iswapDebit(c *chainClient, cfg iswapConfig, s *iswapState) error {
	ids, err := c.contractIDs("@1EditParameter", "@1NewParameter")
	if err != nil {
		return err
	}
	at := "@" + strconv.FormatInt(s.Ecosystem, 10)
	var p struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	}
	var names []string
	err = c.nodes[0].get("ecosystemparam/signer_debit_contracts?ecosystem=1", &p)
	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr) && apiErr.Code == "E_PARAMNOTFOUND":
		p.ID = ""
	case err != nil:
		return fmt.Errorf("signer_debit_contracts: %w", err)
	default:
		for _, name := range strings.Split(p.Value, ",") {
			if name = strings.TrimSpace(name); name != "" && !strings.HasPrefix(name, at) {
				names = append(names, name)
			}
		}
	}
	for _, router := range iswapRouters {
		names = append(names, at+router)
	}
	value := strings.Join(names, ",")
	var tx *signedTx
	if p.ID == "" {
		tx, err = c.signer.contract(c.founder, ids["@1NewParameter"], map[string]any{"Name": "signer_debit_contracts", "Value": value, "Conditions": `ContractConditions("@1DeveloperCondition")`})
	} else {
		pid, perr := strconv.ParseInt(p.ID, 10, 64)
		if perr != nil {
			return fmt.Errorf("signer_debit_contracts: id %q", p.ID)
		}
		tx, err = c.signer.contract(c.founder, ids["@1EditParameter"], map[string]any{"Id": pid, "Value": value})
	}
	_, err = c.one("signer_debit_contracts", tx, err, cfg.timeout)
	return err
}

// iswapAddLiquidity adds the first liquidity of each pair, of the founder
func iswapAddLiquidity(c *chainClient, cfg iswapConfig, s *iswapState) error {
	addID, err := c.iswapID(s, "AddLiquidity")
	if err != nil {
		return err
	}
	// The pairs of a token ecosystem take the tokens of the founder one after another
	for _, p := range s.Pairs {
		tx, err := c.signer.contractIn(c.founder, s.Ecosystem, addID, map[string]any{
			"EcosystemA": p[0], "EcosystemB": p[1], "AmountADesired": iswapLiquidity, "AmountBDesired": iswapLiquidity, "AmountAMin": "1", "AmountBMin": "1", "Deadline": deadline(),
		})
		if _, err := c.one(fmt.Sprintf("AddLiquidity %d-%d", p[0], p[1]), tx, err, cfg.timeout); err != nil {
			return err
		}
	}
	return nil
}

// iswapPair is a row of the table pair of iSwap
type iswapPair struct {
	ID       json.Number `json:"id"`
	Eco1     json.Number `json:"ecosystem1"`
	Eco2     json.Number `json:"ecosystem2"`
	Reserve1 json.Number `json:"reserve1"`
	Reserve2 json.Number `json:"reserve2"`
	Account  string      `json:"pair_account"`
}

// pair reads the pair of two token ecosystems
func (c *chainClient) pair(s *iswapState, p [2]int64) (*iswapPair, error) {
	var r struct {
		List []iswapPair `json:"list"`
	}
	where := fmt.Sprintf(`{"ecosystem1":%d,"ecosystem2":%d}`, p[0], p[1])
	if err := c.nodes[0].get(fmt.Sprintf("list/pair?ecosystem=%d&where=%s", s.Ecosystem, url.QueryEscape(where)), &r); err != nil {
		return nil, err
	}
	if len(r.List) != 1 {
		return nil, fmt.Errorf("pair %d-%d: %d rows", p[0], p[1], len(r.List))
	}
	return &r.List[0], nil
}

// tokenBalance is the tokens of an ecosystem on the account
func (c *chainClient) tokenBalance(address string, eco int64) (string, error) {
	var r struct {
		Amount string `json:"amount"`
	}
	err := c.nodes[0].get("balance/"+url.PathEscape(address)+"?ecosystem="+strconv.FormatInt(eco, 10), &r)
	return r.Amount, err
}

// iswapCheck checks iSwap on the chain: a swap pays what the formula of Uniswap V2 tells and keeps
// the reserves equal to the balances of the pair, a removal of liquidity pays back its share, and
// the contracts refuse the calls that would take the tokens of the pairs
func iswapCheck(c *chainClient, cfg iswapConfig, s *iswapState) error {
	// The headers of a process share their time: InitParam again would be the transaction of the
	// step pairs, which the node takes for a double, when one process ran both
	c.signer.time = time.Now().Unix()
	a := cfg.accounts[0]
	p := s.Pairs[0]
	swapID, err := c.iswapID(s, "SwapExactTokensForTokens")
	if err != nil {
		return err
	}
	before, err := c.pair(s, p)
	if err != nil {
		return err
	}
	in0, err := c.tokenBalance(a.address(), p[0])
	if err != nil {
		return err
	}
	out0, err := c.tokenBalance(a.address(), p[1])
	if err != nil {
		return err
	}
	amountIn := int64(iswapAmountIn)
	tx, err := c.signer.contractIn(a, s.Ecosystem, swapID, map[string]any{"Path": []any{p[0], p[1]}, "AmountIn": strconv.FormatInt(amountIn, 10), "AmountOutMin": "1"})
	if err := c.fail("SwapExactTokensForTokens without a deadline", tx, err, "Parameter Deadline has not been found", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, swapID, map[string]any{"Path": []any{p[0], p[1]}, "AmountIn": strconv.FormatInt(amountIn, 10), "AmountOutMin": "1", "Deadline": 1})
	if err := c.fail("SwapExactTokensForTokens past its deadline", tx, err, "EXPIRED", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, swapID, map[string]any{"Path": []any{p[0], p[1]}, "AmountIn": strconv.FormatInt(amountIn, 10), "AmountOutMin": "1", "Deadline": deadline()})
	result, err := c.one("SwapExactTokensForTokens", tx, err, cfg.timeout)
	if err != nil {
		return err
	}
	after, err := c.pair(s, p)
	if err != nil {
		return err
	}
	in1, _ := c.tokenBalance(a.address(), p[0])
	out1, _ := c.tokenBalance(a.address(), p[1])
	pay0, _ := c.tokenBalance(after.Account, p[0])
	pay1, _ := c.tokenBalance(after.Account, p[1])
	want := amountOut(dec(before.Reserve1.String()), dec(before.Reserve2.String()), dec(strconv.FormatInt(amountIn, 10)))
	got := dec(out1).Sub(dec(out0))
	log.Printf("swap %d of %d for %d: result %s, paid %s, got %s (formula %s); reserves %s/%s -> %s/%s, balances of the pair %s/%s",
		amountIn, p[0], p[1], result, dec(in0).Sub(dec(in1)), got, want, before.Reserve1, before.Reserve2, after.Reserve1, after.Reserve2, pay0, pay1)
	switch {
	case !dec(in0).Sub(dec(in1)).Equal(dec(strconv.FormatInt(amountIn, 10))):
		return fmt.Errorf("the account paid %s, not %d", dec(in0).Sub(dec(in1)), amountIn)
	case !got.Equal(want):
		return fmt.Errorf("the account got %s, the formula tells %s", got, want)
	case !dec(after.Reserve1.String()).Equal(dec(pay0)) || !dec(after.Reserve2.String()).Equal(dec(pay1)):
		return fmt.Errorf("the reserves %s/%s are not the balances of the pair %s/%s", after.Reserve1, after.Reserve2, pay0, pay1)
	}

	// The tokens of a pair go by its contracts only, from the routers only
	at := "@" + strconv.FormatInt(s.Ecosystem, 10)
	ids, err := c.contractIDs(at+"PairSwap", at+"PairBurn", at+"MintFee", "@1TokensSendSeed", "@1TokensTransfer", at+"CreatePair", at+"InitParam")
	if err != nil {
		return err
	}
	// a transaction decodes a map to a map[string]any, which a contract refuses for a parameter of
	// the type map: no transaction calls PairSwap, PairBurn or MintFee, and the conditions refuse the
	// contracts that are not their routers
	pairMap := map[string]any{"id": after.ID.String(), "pair_account": after.Account}
	tx, err = c.signer.contractIn(a, s.Ecosystem, ids[at+"PairSwap"], map[string]any{"Pair": pairMap, "EcosystemIn": p[0], "AmountIn": "1", "AmountOut": "1000000", "To": a.address()})
	if err := c.fail("PairSwap called by an account", tx, err, "invalid map type", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, ids[at+"PairBurn"], map[string]any{"Pair": pairMap, "Liquidity": "1", "NftId": 1})
	if err := c.fail("PairBurn called by an account", tx, err, "invalid map type", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, ids[at+"MintFee"], map[string]any{"Pair": pairMap})
	if err := c.fail("MintFee called by an account", tx, err, "invalid map type", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contract(a, ids["@1TokensSendSeed"], map[string]any{"Amount": "1", "FromAccount": after.Account, "RecipientAccount": a.address(), "FromEcosystem": p[1]})
	if err := c.fail("TokensSendSeed of a pair by an account", tx, err, "Access denied", cfg.timeout); err != nil {
		return err
	}
	// a transaction of @1TokensTransfer is no contract of signer_debit_contracts, nor of the
	// conditions of a pair
	tx, err = c.signer.contract(a, ids["@1TokensTransfer"], map[string]any{"Amount": "1", "SenderAccount": after.Account, "RecipientAccount": a.address(), "Ecosystem": p[1]})
	if err := c.fail("TokensTransfer of a pair by an account", tx, err, "cannot be made", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contract(a, ids["@1TokensTransfer"], map[string]any{"Amount": "1", "SenderAccount": a.address(), "RecipientAccount": after.Account, "Ecosystem": p[1]})
	if err := c.fail("TokensTransfer of the signer by its transaction", tx, err, "cannot be made", cfg.timeout); err != nil {
		return err
	}
	// a contract of iSwap runs in a transaction of its ecosystem only: in another, the unprefixed
	// tables would be those of the other ecosystem
	tx, err = c.signer.contractIn(a, p[0], swapID, map[string]any{"Path": []any{p[0], p[1]}, "AmountIn": "1000", "AmountOutMin": "1", "Deadline": deadline()})
	if err := c.fail("SwapExactTokensForTokens in a token ecosystem", tx, err, "iSwap runs in the transactions of its ecosystem", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, ids[at+"CreatePair"], map[string]any{"Ecosystem1": p[0], "Ecosystem2": s.Ecosystem})
	if err := c.fail("CreatePair by an account", tx, err, "DeveloperCondition", cfg.timeout); err != nil {
		return err
	}
	tx, err = c.signer.contractIn(c.founder, s.Ecosystem, ids[at+"InitParam"], map[string]any{})
	if err := c.fail("InitParam again", tx, err, "iSwap is initialized", cfg.timeout); err != nil {
		return err
	}
	return iswapCheckRemove(c, cfg, s)
}

// iswapCheckRemove adds liquidity of an account to a pair and removes it: it gets back its share,
// at most what it added
func iswapCheckRemove(c *chainClient, cfg iswapConfig, s *iswapState) error {
	a := cfg.accounts[0]
	p := s.Pairs[len(s.Pairs)-1]
	addID, err := c.iswapID(s, "AddLiquidity")
	if err != nil {
		return err
	}
	removeID, err := c.iswapID(s, "RemoveLiquidity")
	if err != nil {
		return err
	}
	b0, _ := c.tokenBalance(a.address(), p[0])
	b1, _ := c.tokenBalance(a.address(), p[1])
	amount := strconv.FormatInt(iswapAddAmount, 10)
	tx, err := c.signer.contractIn(a, s.Ecosystem, addID, map[string]any{"EcosystemA": p[0], "EcosystemB": p[1], "AmountADesired": amount, "AmountBDesired": amount, "AmountAMin": "1", "AmountBMin": "1", "Deadline": deadline()})
	result, err := c.one("AddLiquidity of an account", tx, err, cfg.timeout)
	if err != nil {
		return err
	}
	// the result of a contract is the text of its value: a map prints as map[key:value ...]
	m := regexp.MustCompile(`liquidity:(\d+) nftId:(\d+)`).FindStringSubmatch(result)
	if m == nil {
		return fmt.Errorf("AddLiquidity: result %q", result)
	}
	liquidity := m[1]
	nft, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return fmt.Errorf("AddLiquidity: result %q", result)
	}
	tx, err = c.signer.contractIn(a, s.Ecosystem, removeID, map[string]any{"EcosystemA": p[0], "EcosystemB": p[1], "Liquidity": liquidity, "NftId": nft, "AmountAMin": "1", "AmountBMin": "1", "Deadline": deadline()})
	result, err = c.one("RemoveLiquidity of an account", tx, err, cfg.timeout)
	if err != nil {
		return err
	}
	e0, _ := c.tokenBalance(a.address(), p[0])
	e1, _ := c.tokenBalance(a.address(), p[1])
	lost0, lost1 := dec(b0).Sub(dec(e0)), dec(b1).Sub(dec(e1))
	log.Printf("add and remove liquidity %s of %d-%d: liquidity %s, result %s, the account is short %s/%s", amount, p[0], p[1], liquidity, result, lost0, lost1)
	if lost0.IsNegative() || lost1.IsNegative() {
		return fmt.Errorf("the account got back more than it added: %s/%s", lost0.Neg(), lost1.Neg())
	}
	if lost0.GreaterThan(dec(amount).Div(dec("1000"))) || lost1.GreaterThan(dec(amount).Div(dec("1000"))) {
		return fmt.Errorf("the account got back %s/%s short of what it added, more than 0.1%%", lost0, lost1)
	}
	return nil
}

// dec is a decimal of its text, 0 for a text that is none
func dec(text string) decimal.Decimal {
	d, err := decimal.NewFromString(text)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// amountOut is what a swap of in pays out of a pair of the reserves in and out, after the fee of
// 0.3%, as Uniswap V2 and SwapExactTokensForTokens tell it
func amountOut(reserveIn, reserveOut, in decimal.Decimal) decimal.Decimal {
	withFee := in.Mul(decimal.NewFromInt(997))
	return withFee.Mul(reserveOut).Div(reserveIn.Mul(decimal.NewFromInt(1000)).Add(withFee)).Floor()
}

// iswapWorkloads are the DeFi workloads of iSwap, of the accounts iswap readied:
//
//	swap      SwapExactTokensForTokens on the first -rotate pairs in turn (1 by default), both ways
//	          in turn: with 1, every swap writes the row and the balances of the same pair
//	swaps     SwapExactTokensForTokens on each pair in turn
//	hop       SwapExactTokensForTokens along -rotate paths of 3 tokens in turn (1 by default),
//	          path i of the tokens 3i, 3i+1, 3i+2 (modulo the tokens), both ways in turn: 2 pairs a
//	          swap
//	exactout  SwapTokensForExactTokens on each pair in turn
//	addliq    AddLiquidity of each account to each pair in turn
//	token     @1TokensSend of a token ecosystem from each account to its peer
//	defi      the mix: 4 swaps, 2 hops, 1 exactout, 1 addliq, 2 token in 10
func iswapWorkloads(c *chainClient, accounts []*account, name string, run int, file string) (workload, error) {
	s, err := loadIswap(file)
	if err != nil {
		return workload{}, fmt.Errorf("workload %s: the state of iswap: %w", name, err)
	}
	if s.Step < len(iswapSteps) {
		return workload{}, fmt.Errorf("workload %s: iswap stopped before %s", name, iswapSteps[s.Step].name)
	}
	if len(accounts) < s.Accounts {
		return workload{}, fmt.Errorf("workload %s: iswap readied %d accounts, the file has %d", name, s.Accounts, len(accounts))
	}
	accounts = accounts[:s.Accounts]
	n := len(accounts)
	ids := map[string]int{}
	for _, contract := range []string{"SwapExactTokensForTokens", "SwapTokensForExactTokens", "AddLiquidity"} {
		if ids[contract], err = c.iswapID(s, contract); err != nil {
			return workload{}, err
		}
	}
	if ids["TokensSend"], err = c.nodes[0].contractID("@1TokensSend"); err != nil {
		return workload{}, err
	}
	// amount makes the amounts of a run unlike, as their transactions
	amount := func(base int64, seq int) string { return strconv.FormatInt(base+int64(run)*1_000_000+int64(seq), 10) }
	path := func(p [2]int64, seq int) []any {
		if (seq/n)%2 == 0 {
			return []any{p[0], p[1]}
		}
		return []any{p[1], p[0]}
	}
	t := s.Tokens
	// the transactions of a run may wait long in the pools: their deadline is far
	until := deadline()
	rotate := max(iswapRotate, 1)
	pairs := s.Pairs[:min(rotate, len(s.Pairs))]
	swap := func(a *account, seq int, p []any) (*signedTx, error) {
		return c.signer.contractIn(a, s.Ecosystem, ids["SwapExactTokensForTokens"], map[string]any{"Path": p, "AmountIn": amount(iswapAmountIn, seq), "AmountOutMin": "1", "Deadline": until})
	}
	kinds := map[string]func(a *account, seq int) (*signedTx, error){
		"swap": func(a *account, seq int) (*signedTx, error) {
			return swap(a, seq, path(pairs[seq%len(pairs)], seq/len(pairs)))
		},
		"swaps": func(a *account, seq int) (*signedTx, error) {
			return swap(a, seq, path(s.Pairs[seq%len(s.Pairs)], seq/len(s.Pairs)))
		},
		"hop": func(a *account, seq int) (*signedTx, error) {
			i := seq % rotate
			hop := []any{t[3*i%len(t)], t[(3*i+1)%len(t)], t[(3*i+2)%len(t)]}
			if (seq/rotate/n)%2 == 1 {
				hop[0], hop[2] = hop[2], hop[0]
			}
			return swap(a, seq, hop)
		},
		"exactout": func(a *account, seq int) (*signedTx, error) {
			p := s.Pairs[seq%len(s.Pairs)]
			return c.signer.contractIn(a, s.Ecosystem, ids["SwapTokensForExactTokens"], map[string]any{"Path": path(p, seq/len(s.Pairs)), "AmountOut": amount(iswapAmountIn/2, seq), "AmountInMax": strconv.FormatInt(iswapAmountIn*10, 10), "Deadline": until})
		},
		"addliq": func(a *account, seq int) (*signedTx, error) {
			p := s.Pairs[seq%len(s.Pairs)]
			return c.signer.contractIn(a, s.Ecosystem, ids["AddLiquidity"], map[string]any{"EcosystemA": p[0], "EcosystemB": p[1], "AmountADesired": amount(iswapAddAmount, seq), "AmountBDesired": strconv.FormatInt(iswapAddAmount*2, 10), "AmountAMin": "1", "AmountBMin": "1", "Deadline": until})
		},
		"token": func(a *account, seq int) (*signedTx, error) {
			return c.signer.contract(a, ids["TokensSend"], map[string]any{"Recipient": accounts[(seq+n/2)%n].address(), "Amount": "1", "Comment": strconv.Itoa(run) + "-" + strconv.Itoa(seq), "Ecosystem": t[seq%len(t)]})
		},
	}
	mix := []string{"swaps", "hop", "swaps", "token", "exactout", "swaps", "hop", "addliq", "swaps", "token"}
	kinds["defi"] = func(a *account, seq int) (*signedTx, error) { return kinds[mix[seq%len(mix)]](a, seq) }
	kind, ok := kinds[strings.TrimPrefix(name, "iswap-")]
	if !ok {
		return workload{}, fmt.Errorf("no workload %q", name)
	}
	if len(t) < 3 && (name == "iswap-hop" || name == "iswap-defi") {
		return workload{}, fmt.Errorf("workload %s: a path of 3 tokens, iswap made %d", name, len(t))
	}
	return workload{name, func(seq int) (*signedTx, error) { return kind(accounts[seq%n], seq) }}, nil
}
