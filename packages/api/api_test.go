/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/assert"
)

var apiAddress = "http://localhost:7079"

var (
	gAuth             string
	gAddress          string
	gPrivate, gPublic string
	gMobile           bool
)

// PrivateToPublicHex returns the hex public key for the specified hex private key.
func PrivateToPublicHex(hexkey string) (string, error) {
	key, err := hex.DecodeString(hexkey)
	if err != nil {
		return ``, fmt.Errorf("Decode hex error")
	}
	pubKey, err := crypto.PrivateToPublic(key)
	if err != nil {
		return ``, err
	}
	return crypto.PubToHex(pubKey), nil
}

func sendRawRequest(rtype, url string, form *url.Values) ([]byte, error) {
	client := &http.Client{}
	var ioform io.Reader
	if form != nil {
		ioform = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(rtype, apiAddress+consts.ApiPath+url, ioform)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if len(gAuth) > 0 {
		req.Header.Set("Authorization", jwtPrefix+gAuth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`%d %s`, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return data, nil
}

func sendRequest(rtype, url string, form *url.Values, v any) error {
	data, err := sendRawRequest(rtype, url, form)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

func sendGet(url string, form *url.Values, v any) error {
	return sendRequest("GET", url, form, v)
}

func sendPost(url string, form *url.Values, v any) error {
	return sendRequest("POST", url, form, v)
}

func keyLogin(state int64) (err error) {
	var (
		key, sign []byte
	)

	key, err = os.ReadFile(`key`)
	if err != nil {
		return
	}
	if len(key) > 64 {
		key = key[:64]
	}
	var ret getUIDResult
	err = sendGet(`getuid`, nil, &ret)
	if err != nil {
		return
	}
	gAuth = ret.Token
	if len(ret.UID) == 0 {
		return fmt.Errorf(`getuid has returned empty uid`)
	}

	var pub string

	sign, err = crypto.SignString(string(key), `LOGIN`+ret.NetworkID+ret.UID)
	if err != nil {
		return
	}
	pub, err = PrivateToPublicHex(string(key))
	if err != nil {
		return
	}
	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)},
		`ecosystem`: {converter.Int64ToStr(state)}, "role_id": {"0"}}
	if gMobile {
		form[`mobile`] = []string{`true`}
	}
	var logret loginResult
	err = sendPost(`login`, &form, &logret)
	if err != nil {
		return
	}
	gAddress = logret.Account
	gPrivate = string(key)
	gPublic, err = PrivateToPublicHex(gPrivate)
	gAuth = logret.Token
	if err != nil {
		return
	}
	return
}

func keyLoginToken(state int64) (err error) {
	var (
		key, sign []byte
	)

	str, _ := os.Getwd()
	fmt.Println("dir " + str)
	key, err = ioutil.ReadFile(`key`)
	if err != nil {
		return
	}
	if len(key) > 64 {
		key = key[:64]
	}
	var ret getUIDResult
	err = sendGet(`getuid`, nil, &ret)
	if err != nil {
		return
	}
	gAuth = ret.Token
	if len(ret.UID) == 0 {
		return fmt.Errorf(`getuid has returned empty uid`)
	}

	var pub string

	sign, err = crypto.SignString(string(key), `LOGIN`+ret.NetworkID+ret.UID)
	if err != nil {
		return
	}
	pub, err = PrivateToPublicHex(string(key))
	if err != nil {
		return
	}
	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)},
		`ecosystem`: {converter.Int64ToStr(state)}, "role_id": {"0"}, "expire": {"5"}}
	if gMobile {
		form[`mobile`] = []string{`true`}
	}
	var logret loginResult
	err = sendPost(`login`, &form, &logret)
	if err != nil {
		return
	}
	gAddress = logret.Account
	gPrivate = string(key)
	gPublic, err = PrivateToPublicHex(gPrivate)
	gAuth = logret.Token
	if err != nil {
		return
	}
	return
}

func keyLoginex(state int64, m ...string) (err error) {
	var (
		key, sign []byte
	)

	key, err = ioutil.ReadFile(`key` + m[0])
	if err != nil {
		return
	}
	if len(key) > 64 {
		key = key[:64]
	}
	var ret getUIDResult
	err = sendGet(`getuid`, nil, &ret)
	if err != nil {
		return
	}
	gAuth = ret.Token
	if len(ret.UID) == 0 {
		return fmt.Errorf(`getuid has returned empty uid`)
	}

	var pub string

	sign, err = crypto.SignString(string(key), `LOGIN`+ret.NetworkID+ret.UID)
	if err != nil {
		return
	}
	pub, err = PrivateToPublicHex(string(key))
	if err != nil {
		return
	}
	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)},
		`ecosystem`: {converter.Int64ToStr(state)}, "role_id": {"0"}}
	if gMobile {
		form[`mobile`] = []string{`true`}
	}
	var logret loginResult
	err = sendPost(`login`, &form, &logret)
	if err != nil {
		return
	}
	gAddress = logret.Account
	gPrivate = string(key)
	gPublic, err = PrivateToPublicHex(gPrivate)
	gAuth = logret.Token
	if err != nil {
		return
	}
	return
}

func waitTx(hash string) (blockid int64, penalty int64, err error) {
	data, err := json.Marshal(&txstatusRequest{
		Hashes: []string{hash},
	})
	if err != nil {
		return
	}

	for i := 0; i < 100; i++ {
		var multiRet multiTxStatusResult
		err = sendPost(`txstatus`, &url.Values{
			"data": {string(data)},
		}, &multiRet)
		if err != nil {
			return
		}

		ret := multiRet.Results[hash]
		var errtext []byte
		if len(ret.BlockID) > 0 {
			blockid = converter.StrToInt64(ret.BlockID)
			penalty = ret.Penalty
			if ret.Penalty == 1 {
				errtext, err = json.Marshal(ret.Message)
				if err != nil {
					return
				}
				err = errors.New(string(errtext))
				return
			} else {
				err = fmt.Errorf(ret.Result)
				return
			}
		}
		if ret.Message != nil {
			errtext, err = json.Marshal(ret.Message)
			if err != nil {
				return
			}
			err = errors.New(string(errtext))
			return
		}
		time.Sleep(time.Second)
	}

	return 0, 0, fmt.Errorf(`TxStatus timeout`)
}

func randName(prefix string) string {
	return fmt.Sprintf(`%s%d`, prefix, time.Now().Unix())
}

type getter interface {
	Get(string) string
}

type contractParams map[string]any

func (cp *contractParams) Get(key string) string {
	if _, ok := (*cp)[key]; !ok {
		return ""
	}
	return fmt.Sprintf("%v", (*cp)[key])
}

func (cp *contractParams) GetRaw(key string) any {
	return (*cp)[key]
}

func postTxResult(name string, form getter) (id int64, msg string, err error) {
	var contract getContractResult
	if err = sendGet("contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]any)
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int", "address":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		case "array":
			var v any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "string", "money":
			params[name] = value
		case "file", "bytes":
			if cp, ok := form.(*contractParams); !ok {
				err = fmt.Errorf("Form is not *contractParams type")
			} else {
				params[name] = cp.GetRaw(name)
			}
		}

		if err != nil {
			err = fmt.Errorf("Parse param '%s': %s", name, err)
			return
		}
	}

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}

	data, hash, err := transaction.NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          int(contract.ID),
			EcosystemID: 1,
			Time:        time.Now().Unix(),
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: params,
		Lang:   "en",
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart("sendTx", map[string][]byte{
		hex.EncodeToString(hash): data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, penalty, err := waitTx(ret.Hashes[hex.EncodeToString(hash)])
	if id != 0 && err != nil {
		if penalty == 1 {
			return
		}
		msg = err.Error()
		err = nil
	}

	return
}

func postTxResultMultipart(name string, form getter) (id int64, msg string, err error) {
	var contract getContractResult
	if err = sendGet("contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]any)
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int", "address":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		case "array":
			var v any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "string", "money":
			params[name] = value
		case "file", "bytes":
			if cp, ok := form.(*contractParams); !ok {
				err = fmt.Errorf("Form is not *contractParams type")
			} else {
				params[name] = cp.GetRaw(name)
			}
		}

		if err != nil {
			err = fmt.Errorf("Parse param '%s': %s", name, err)
			return
		}
	}

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}
	arrData := make(map[string][]byte)

	for i := 0; i < 1; i++ {
		conname := crypto.RandSeq(10)
		params["ApplicationId"] = int64(1)
		params["Conditions"] = "1"
		//params["TokenEcosystem"] = int64(2)
		params["Value"] = fmt.Sprintf(`contract rnd%v%d  { action { }}`, conname, i)
		expedite := strconv.Itoa(1)
		data, txhash, _ := transaction.NewTransactionInProc(types.SmartTransaction{
			Header: &types.Header{
				ID:          int(contract.ID),
				Time:        time.Now().Unix(),
				EcosystemID: 1,
				KeyID:       crypto.Address(publicKey),
				NetworkID:   conf.Config.LocalConf.NetworkID,
			},
			Params:   params,
			Expedite: expedite,
		}, privateKey)
		arrData[fmt.Sprintf("%x", txhash)] = data
		fmt.Println(fmt.Sprintf("%x", txhash))
	}
	ret := &sendTxResult{}
	err = sendMultipart("sendTx", arrData, &ret)
	//err = sendMultipart("sendTx", map[string][]byte{
	//	"data": data,
	//}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}

	//var ids, ps []int64
	//
	//for s := range arrData {
	//	id, penalty, err := waitTx(ret.Hashes[s])
	//	ids = append(ids, id)
	//	ps = append(ps, penalty)
	//	if id != 0 && err != nil {
	//		if penalty == 1 {
	//			//return
	//		}
	//		msg = err.Error()
	//		err = nil
	//	}
	//}
	//fmt.Println(ids, ps)

	return
}

func postSignTxResult(name string, form getter) (id int64, msg string, err error) {
	var contract getContractResult
	if err = sendGet("contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]any)
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int", "address":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		case "array":
			var v any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "string", "money":
			params[name] = value
		case "file", "bytes":
			if cp, ok := form.(*contractParams); !ok {
				err = fmt.Errorf("Form is not *contractParams type")
			} else {
				params[name] = cp.GetRaw(name)
			}
		}

		if err != nil {
			err = fmt.Errorf("Parse param '%s': %s", name, err)
			return
		}
	}

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}

	data, _, err := transaction.NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          int(contract.ID),
			EcosystemID: 1,
			Time:        time.Now().Unix(),
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: params,
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart("sendSignTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, penalty, err := waitTx(ret.Hashes["data"])
	if id != 0 && err != nil {
		if penalty == 1 {
			return
		}
		msg = err.Error()
		err = nil
	}
	return
}

func postTxResult2(name string, form getter) (id int64, msg string, err error) {
	var contract getContractResult
	if err = sendGet("contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]any)
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int", "address":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		case "array":
			var v any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]any
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "string", "money":
			params[name] = value
		case "file", "bytes":
			if cp, ok := form.(*contractParams); !ok {
				err = fmt.Errorf("Form is not *contractParams type")
			} else {
				params[name] = cp.GetRaw(name)
			}
		}

		if err != nil {
			err = fmt.Errorf("Parse param '%s': %s", name, err)
			return
		}
	}

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}

	data, _, err := transaction.NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          int(contract.ID),
			EcosystemID: 2,
			Time:        time.Now().Unix(),
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: params,
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart("sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, penalty, err := waitTx(ret.Hashes["data"])
	if id != 0 && err != nil {
		if penalty == 1 {
			return
		}
		msg = err.Error()
		err = nil
	}
	return
}

func RawToString(input json.RawMessage) string {
	out := strings.Trim(string(input), `"`)
	return strings.Replace(out, `\"`, `"`, -1)
}

func postTx(txname string, form *url.Values) error {
	_, _, err := postTxResult(txname, form)
	return err
}

func postTxMultipart(txname string, form *url.Values) error {
	_, _, err := postTxResultMultipart(txname, form)
	return err
}

func postTransferSelfTxMultipart(form *url.Values) error {
	_, _, err := postTransferSelfTxResult(form)
	return err
}

func postUTXOTxMultipart(form *url.Values) error {
	_, _, err := postUTXOTxResult(form)
	return err
}

func postTransferSelfTxResult(form getter) (id int64, msg string, err error) {

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}

	data, _, err := transaction.NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          int(1),
			EcosystemID: 1,
			Time:        time.Now().Unix(),
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		TransferSelf: &types.TransferSelf{
			Value: "1000000000000000000",
			//Asset:  "IBAX",
			Source: "UTXO",
			Target: "Account",
		},
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart("sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, penalty, err := waitTx(ret.Hashes["data"])
	if id != 0 && err != nil {
		if penalty == 1 {
			return
		}
		msg = err.Error()
		err = nil
	}
	return
}

func postUTXOTxResult(form getter) (id int64, msg string, err error) {

	var privateKey, publicKey []byte
	if privateKey, err = hex.DecodeString(gPrivate); err != nil {
		return
	}
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		return
	}

	data, _, err := transaction.NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          int(1),
			EcosystemID: 1,
			Time:        time.Now().Unix(),
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		UTXO: &types.UTXO{
			Value: "1000000000000000",
			ToID:  -8055926748644556208,
		},
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart("sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, penalty, err := waitTx(ret.Hashes["data"])
	if id != 0 && err != nil {
		if penalty == 1 {
			return
		}
		msg = err.Error()
		err = nil
	}
	return
}

func postSignTx(txname string, form *url.Values) error {
	_, _, err := postSignTxResult(txname, form)
	return err
}

func cutErr(err error) string {
	out := err.Error()
	if off := strings.IndexByte(out, '('); off != -1 {
		out = out[:off]
	}
	return strings.TrimSpace(out)
}

func TestGetAvatar(t *testing.T) {

	err := keyLogin(1)
	assert.NoError(t, err)

	url := `http://localhost:7079` + consts.ApiPath + "avatar/-1744264011260937456"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	assert.NoError(t, err)

	if len(gAuth) > 0 {
		req.Header.Set("Authorization", jwtPrefix+gAuth)
	}

	cli := http.DefaultClient
	resp, err := cli.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()
	mime := resp.Header.Get("Content-Type")
	expectedMime := "image/png"
	assert.Equal(t, expectedMime, mime, "content type must be a '%s' but returns '%s'", expectedMime, mime)
}

func sendMultipart(url string, files map[string][]byte, v any) error {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	for key, data := range files {
		part, err := writer.CreateFormFile(key, key)
		if err != nil {
			return err
		}
		if _, err := part.Write(data); err != nil {
			return err
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiAddress+consts.ApiPath+url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if len(gAuth) > 0 {
		req.Header.Set("Authorization", jwtPrefix+gAuth)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(`%d %s`, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return json.Unmarshal(data, &v)
}
