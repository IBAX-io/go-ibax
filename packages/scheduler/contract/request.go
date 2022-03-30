/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package contract

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

const (
	headerAuthPrefix = "Bearer "
)

type authResult struct {
	UID   string `json:"uid,omitempty"`
	Token string `json:"token,omitempty"`
}

type contractResult struct {
	Hash string `json:"hash"`
	// These fields are used for CLB
	Message struct {
		Type  string `json:"type,omitempty"`
		Error string `json:"error,omitempty"`
	} `json:"errmsg,omitempty"`
	Result string `json:"result,omitempty"`
}

// NodeContract creates a transaction to execute the contract.
// The transaction is signed with a node key.
func NodeContract(Name string) (result contractResult, err error) {
	var (
		sign                          []byte
		ret                           authResult
		NodePrivateKey, NodePublicKey string
	)
	err = sendAPIRequest(`GET`, `getuid`, nil, &ret, ``)
	if err != nil {
		return
	}
	auth := ret.Token
	if len(ret.UID) == 0 {
		err = fmt.Errorf(`getuid has returned empty uid`)
		return
	}
	NodePrivateKey, NodePublicKey = utils.GetNodeKeys()
	if len(NodePrivateKey) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		err = errors.New(`empty node private key`)
		return
	}
	sign, err = crypto.SignString(NodePrivateKey, ret.UID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing node uid")
		return
	}
	form := url.Values{"pubkey": {NodePublicKey}, "signature": {hex.EncodeToString(sign)},
		`ecosystem`: {converter.Int64ToStr(1)}}
	var logret authResult
	err = sendAPIRequest(`POST`, `login`, &form, &logret, auth)
	if err != nil {
		return
	}
	auth = logret.Token
	form = url.Values{`clb`: {`true`}}
	err = sendAPIRequest(`POST`, `node/`+Name, &form, &result, auth)
	if err != nil {
		return
	}
	return
}

func sendAPIRequest(rtype, url string, form *url.Values, v any, auth string) error {
	client := &http.Client{}
	var ioform io.Reader
	if form != nil {
		ioform = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(rtype, fmt.Sprintf(`http://%s:%d%s%s`, conf.Config.HTTP.Host,
		conf.Config.HTTP.Port, consts.ApiPath, url), ioform)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("new api request")
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if len(auth) > 0 {
		req.Header.Set("Authorization", headerAuthPrefix+auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("api request")
		return err
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading api answer")
		return err
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("api status code")
		return fmt.Errorf(`%d %s`, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if err = json.Unmarshal(data, v); err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling api answer")
		return err
	}
	return nil
}
