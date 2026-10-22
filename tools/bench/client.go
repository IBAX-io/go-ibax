/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// apiError is an error the API answered with
type apiError struct {
	Code string `json:"error"`
	Msg  string `json:"msg"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Msg }

// node is a client of the API of a node, logged in with the founder key: a session submits the
// transactions of any account, and reads the blocks
type node struct {
	api   string
	token string
	http  *http.Client
}

// chainInfo is what a node tells of its network on /getuid
type chainInfo struct {
	NetworkID int64
	Cryptoer  string
	Hasher    string
}

func newNode(base string) *node {
	return &node{
		api: strings.TrimRight(base, "/") + "/api/v2/",
		http: &http.Client{Timeout: time.Minute, Transport: &http.Transport{
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     time.Minute,
		}},
	}
}

func (n *node) do(method, path, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, n.api+path, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}
	resp, err := n.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e apiError
		if json.Unmarshal(data, &e) == nil && e.Code != "" {
			return fmt.Errorf("%s %s: %w", method, path, &e)
		}
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	return nil
}

func (n *node) get(path string, out any) error { return n.do(http.MethodGet, path, "", nil, out) }

func (n *node) post(path string, form url.Values, out any) error {
	return n.do(http.MethodPost, path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), out)
}

// login opens a session of ecosystem 1 with the key and tells the network of the node
func (n *node) login(key *account) (*chainInfo, error) {
	var uid struct {
		UID       string `json:"uid"`
		Token     string `json:"token"`
		NetworkID string `json:"network_id"`
		Cryptoer  string `json:"cryptoer"`
		Hasher    string `json:"hasher"`
	}
	if err := n.get("getuid", &uid); err != nil {
		return nil, err
	}
	networkID, err := strconv.ParseInt(uid.NetworkID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("network id %q: %w", uid.NetworkID, err)
	}
	info := &chainInfo{NetworkID: networkID, Cryptoer: uid.Cryptoer, Hasher: uid.Hasher}
	if err := useChain(info); err != nil {
		return nil, err
	}
	signature, err := crypto.AccountSign(key.algo, key.private, []byte("LOGIN"+uid.NetworkID+uid.UID))
	if err != nil {
		return nil, err
	}
	n.token = uid.Token
	var session struct {
		Token string `json:"token"`
	}
	if err := n.post("login", url.Values{
		"pubkey":    {hex.EncodeToString(key.public.Bytes())},
		"signature": {hex.EncodeToString(signature)},
		"ecosystem": {"1"},
	}, &session); err != nil {
		return nil, err
	}
	n.token = session.Token
	return info, nil
}

// useChain sets the hash of the network, which the transactions and their signatures use
func useChain(info *chainInfo) error {
	if _, ok := crypto.HashAlgo_value[info.Hasher]; !ok {
		return fmt.Errorf("the network hashes with %q, which go-ibax does not implement", info.Hasher)
	}
	if _, ok := crypto.AsymAlgo_value[info.Cryptoer]; !ok {
		return fmt.Errorf("the network's node keys are %q, which go-ibax does not implement", info.Cryptoer)
	}
	crypto.InitHashAlgo(info.Hasher)
	crypto.InitAsymAlgo(info.Cryptoer)
	return nil
}

// contractID is the id of a contract, which the header of its transactions names
func (n *node) contractID(name string) (int, error) {
	var info struct {
		ID int `json:"id"`
	}
	if err := n.get("contract/"+url.PathEscape(name), &info); err != nil {
		return 0, err
	}
	return info.ID, nil
}

// send submits transactions in one request; each is its hash and its bytes
func (n *node) send(txs []*signedTx) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, tx := range txs {
		if err := w.WriteField(tx.hash, hex.EncodeToString(tx.data)); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return n.do(http.MethodPost, "sendTx", w.FormDataContentType(), &body, nil)
}

// maxBlock is the id of the last block of the node
func (n *node) maxBlock() (int64, error) {
	var r struct {
		MaxBlockID int64 `json:"max_block_id"`
	}
	err := n.get("maxblockid", &r)
	return r.MaxBlockID, err
}

// blockTxs are the hashes, in hex, of the transactions of the blocks from to to
func (n *node) blockTxs(from, to int64) (map[int64][]string, error) {
	var r map[int64][]struct {
		Hash []byte `json:"hash"`
	}
	if err := n.get(fmt.Sprintf("blocks?block_id=%d&count=%d", from, to-from+1), &r); err != nil {
		return nil, err
	}
	out := make(map[int64][]string, len(r))
	for id, txs := range r {
		for _, tx := range txs {
			out[id] = append(out[id], hex.EncodeToString(tx.Hash))
		}
	}
	return out, nil
}

// txError is the refusal of a transaction
type txError struct {
	Type string `json:"type"`
	Msg  string `json:"error"`
}

// txStatus is what the node tells of a transaction it has seen: the block it is in, if any, and
// its refusal, if the node refused it or it failed in its block
type txStatus struct {
	BlockID string   `json:"blockid"`
	Message *txError `json:"errmsg"`
	Result  string   `json:"result"`
}

// done is whether the transaction succeeded in a block, or failed for good
func (s *txStatus) done() bool { return s.Message != nil || (s.BlockID != "" && s.BlockID != "0") }

// failure is why the transaction failed, or "" if it did not
func (s *txStatus) failure() string {
	if s.Message == nil {
		return ""
	}
	return s.Message.Type + ": " + s.Message.Msg
}

// statuses are the statuses of transactions; a transaction the node never saw has none
func (n *node) statuses(hashes []string) (map[string]*txStatus, error) {
	out, err := n.statusesOf(hashes)
	var e *apiError
	if !errors.As(err, &e) || e.Code != "E_HASHNOTFOUND" {
		return out, err
	}
	// The node answers no status to a request one hash of which it never saw: each by itself
	out = map[string]*txStatus{}
	for _, h := range hashes {
		r, err := n.statusesOf([]string{h})
		if errors.As(err, &e) && e.Code == "E_HASHNOTFOUND" {
			continue
		}
		if err != nil {
			return nil, err
		}
		maps.Copy(out, r)
	}
	return out, nil
}

func (n *node) statusesOf(hashes []string) (map[string]*txStatus, error) {
	request, err := json.Marshal(map[string][]string{"hashes": hashes})
	if err != nil {
		return nil, err
	}
	var r struct {
		Results map[string]*txStatus `json:"results"`
	}
	err = n.post("txstatus", url.Values{"data": {string(request)}}, &r)
	return r.Results, err
}

// balance is the coins of ecosystem 1 of an account: its account and its UTXO
func (n *node) balance(address string) (amount, utxo string, err error) {
	var r struct {
		Amount string `json:"amount"`
		Utxo   string `json:"utxo"`
	}
	err = n.get("balance/"+url.PathEscape(address)+"?ecosystem=1", &r)
	return r.Amount, r.Utxo, err
}

// metrics is the text of the Prometheus metrics of the node, at the URL given
func scrape(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
