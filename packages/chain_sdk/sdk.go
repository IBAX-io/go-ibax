/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain_sdk

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto"
)

//const apiAddress = "http://localhost:7079"
var (
	//ApiAddress     string
	//ApiEcosystemID int64
	ApiPrivateFor []string
	//ApiAuth       string
)

var (
//gAuth             string
//gAddress          string
//gPrivate, gPublic string
//gMobile bool
)

type global struct {
	url   string
	value string
}

// PrivateToPublicHex returns the hex public key for the specified hex private key.
func PrivateToPublicHex(hexkey string) (string, error) {
	key, err := hex.DecodeString(hexkey)
	if err != nil {
		return ``, fmt.Errorf("Decode hex error")
	}
	pubKey, err := crypto.PrivateToPublic(key)
	if err != nil {
		return ``, err
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

func sendRequest(apiAddress string, gAuth string, rtype, url string, form *url.Values, v interface{}) error {
	data, err := sendRawRequest(apiAddress, gAuth, rtype, url, form)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

func sendGet(apiAddress string, gAuth string, url string, form *url.Values, v interface{}) error {
	return sendRequest(apiAddress, gAuth, "GET", url, form, v)
}

func sendPost(apiAddress string, gAuth string, url string, form *url.Values, v interface{}) error {
	return sendRequest(apiAddress, gAuth, "POST", url, form, v)
}

//func keyLogin(apiAddress string, state int64) (err error) {
//	var (
//		key, sign []byte
//	)
//
//	key, err = os.ReadFile(`key`)
//	if err != nil {
//		return
//	}
//	if len(key) > 64 {
//		key = key[:64]
//	}
//	var ret getUIDResult
//	err = sendGet(apiAddress, `getuid`, nil, &ret)
//	if err != nil {
//		return
//	}
//	gAuth = ret.Token
//	if len(ret.UID) == 0 {
//		return fmt.Errorf(`getuid has returned empty uid`)
//	}
//
//	var pub string
//
//	sign, err = crypto.SignString(string(key), nonceSalt+ret.UID)
//	if err != nil {
//		return
//	}
//	pub, err = PrivateToPublicHex(string(key))
//	if err != nil {
//		return
//	}
//	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)},
//		`ecosystem`: {converter.Int64ToStr(state)}, "role_id": {"0"}}
//	if gMobile {
//		form[`mobile`] = []string{`true`}
//	}
//	var logret loginResult
//	err = sendPost(apiAddress, `login`, &form, &logret)
//	if err != nil {
//		return
//	}
//	gAddress = logret.Address
//	gPrivate = string(key)
//	gPublic, err = PrivateToPublicHex(gPrivate)
//	gAuth = logret.Token
//	if err != nil {
//		return
//	}
//	return
//}

func getSign(forSign string, gPrivate string) (string, error) {
	sign, err := crypto.SignString(gPrivate, forSign)
	if err != nil {
		return ``, err
	}
	return hex.EncodeToString(sign), nil
}

func appendSign(gPrivate string, ret map[string]interface{}, form *url.Values) error {
	forsign := ret[`forsign`].(string)
	if ret[`signs`] != nil {
		for _, item := range ret[`signs`].([]interface{}) {
			v := item.(map[string]interface{})
			vsign, err := getSign(gPrivate, v[`forsign`].(string))
			if err != nil {
				return err
			}
			(*form)[v[`field`].(string)] = []string{vsign}
			forsign += `,` + vsign
		}
	}
	sign, err := getSign(forsign, gPrivate)
	if err != nil {
		return err
	}
	(*form)[`time`] = []string{ret[`time`].(string)}
	(*form)[`signature`] = []string{sign}
	return nil
}

func waitTx(apiAddress string, gAuth string, hash string) (int64, error) {
	data, err := json.Marshal(&txstatusRequest{
		Hashes: []string{hash},
	})
	if err != nil {
		return 0, err
	}

	for i := 0; i < 15; i++ {
		var multiRet multiTxStatusResult
		err := sendPost(apiAddress, gAuth, `txstatus`, &url.Values{
			"data": {string(data)},
		}, &multiRet)
		if err != nil {
			return 0, err
		}

		ret := multiRet.Results[hash]

		if len(ret.BlockID) > 0 {
			return converter.StrToInt64(ret.BlockID), fmt.Errorf(ret.Result)
		}
		if ret.Message != nil {
			errtext, err := json.Marshal(ret.Message)
			if err != nil {
				return 0, err
			}
			return 0, errors.New(string(errtext))
		}
		time.Sleep(time.Second)
	}
	return 0, fmt.Errorf(`TxStatus timeout`)
}

func randName(prefix string) string {
	return fmt.Sprintf(`%s%d`, prefix, time.Now().Unix())
}

type getter interface {
	Get(string) string
}

type contractParams map[string]interface{}

func (cp *contractParams) Get(key string) string {
	if _, ok := (*cp)[key]; !ok {
		return ""
	}
	return fmt.Sprintf("%v", (*cp)[key])
}

func (cp *contractParams) GetRaw(key string) interface{} {
	return (*cp)[key]
}

func postTxResult(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, name string, form getter) (id int64, msg string, err error) {
	var contract getContractResult
	if err = sendGet(apiAddress, gAuth, "contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]interface{})
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
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

	/*data, _, err := tx.NewTransaction(tx.SmartContract{
		Header: tx.Header{
			ID:          int(contract.ID),
			Time:        time.Now().Unix(),
			EcosystemID: 1,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   consts.NETWORK_ID,
		},
		Params: params,
	}, privateKey)*/
	data, _, err := NewTransaction(SmartContract{
		Header: Header{
			ID:   int(contract.ID),
			Time: time.Now().Unix(),
			//EcosystemID: 1,
			EcosystemID: apiEcosystemID,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.NetworkID,
			PrivateFor:  ApiPrivateFor,
		},
		Params: params,
	}, privateKey)
	if err != nil {
		return 0, "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart(apiAddress, gAuth, "sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	if len(form.Get("nowait")) > 0 {
		return
	}
	id, err = waitTx(apiAddress, gAuth, ret.Hashes["data"])
	if id != 0 && err != nil {
		msg = err.Error()
		err = nil
	}

	return
}

func RawToString(input json.RawMessage) string {
	out := strings.Trim(string(input), `"`)
	return strings.Replace(out, `\"`, `"`, -1)
}

func postTx(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, txname string, form *url.Values) error {
	_, _, err := postTxResult(apiAddress, apiEcosystemID, gAuth, gPrivate, txname, form)
	return err
}

func cutErr(err error) string {
	out := err.Error()
	if off := strings.IndexByte(out, '('); off != -1 {
		out = out[:off]
	}
	return strings.TrimSpace(out)
}

//func TestGetAvatar(t *testing.T) {
//
//	err := keyLogin("http://localhost:7079", 1)
//	assert.NoError(t, err)
//
//	url := `http://localhost:7079` + consts.ApiPath + "avatar/-1744264011260937456"
//	req, err := http.NewRequest(http.MethodGet, url, nil)
//	assert.NoError(t, err)
//
//	if len(gAuth) > 0 {
//		req.Header.Set("Authorization", jwtPrefix+gAuth)
//	}
//
//	cli := http.DefaultClient
//	resp, err := cli.Do(req)
//	assert.NoError(t, err)
//
//	defer resp.Body.Close()
//	mime := resp.Header.Get("Content-Type")
//	expectedMime := "image/png"
//	assert.Equal(t, expectedMime, mime, "content type must be a '%s' but returns '%s'", expectedMime, mime)
//}

func sendMultipart(ApiAddress string, gAuth string, url string, files map[string][]byte, v interface{}) error {
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

	req, err := http.NewRequest("POST", ApiAddress+consts.ApiPath+url, body)
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

// ADD
func KeyLogin(apiAddress string, from string, state int64) (gAuth string, gAddress string, gPrivate string, gPublic string, gMobile bool, err error) {
	var (
		key, sign []byte
	)

	key, err = os.ReadFile(from)
	if err != nil {
		return "", "", "", "", false, err
	}
	if len(key) > 64 {
		key = key[:64]
	}

	// add  get new uid
	gAuth = ""

	var ret getUIDResult
	err = sendGet(apiAddress, gAuth, `getuid`, nil, &ret)
	if err != nil {
		return "", "", "", "", false, err
	}
	gAuth = ret.Token

	if len(ret.UID) == 0 {
		return "", "", "", "", false, fmt.Errorf(`getuid has returned empty uid`)
	}

	var pub string

	//sign, err = crypto.SignString(string(key), nonceSalt+ret.UID)
	//if err != nil {
	//	return
	//}
	sign, err = crypto.SignString(string(key), `LOGIN`+ret.NetworkID+ret.UID)
	if err != nil {
		return "", "", "", "", false, err
	}

	pub, err = PrivateToPublicHex(string(key))
	if err != nil {
		return "", "", "", "", false, err
	}
	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)},
		`ecosystem`: {converter.Int64ToStr(state)}, "role_id": {"0"}}
	if gMobile {
		form[`mobile`] = []string{`true`}
	}
	var logret loginResult
	err = sendPost(apiAddress, gAuth, `login`, &form, &logret)
	if err != nil {
		return "", "", "", "", false, err
	}
	gAddress = logret.Address
	gPrivate = string(key)
	gPublic, err = PrivateToPublicHex(gPrivate)
	gAuth = logret.Token
	//
	//ApiAuth = gAuth

	if err != nil {
		return "", "", "", "", false, err
	}
	return gAuth, gAddress, gPrivate, gPublic, gMobile, nil
}

func SendGet(apiAddress string, gAuth string, url string, form *url.Values, v interface{}) error {
	return sendRequest(apiAddress, gAuth, "GET", url, form, v)
}

func SendPost(apiAddress string, gAuth string, url string, form *url.Values, v interface{}) error {
	return sendRequest(apiAddress, gAuth, "POST", url, form, v)
}

func PostTx(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, txname string, form *url.Values) error {
	_, _, err := postTxResult(apiAddress, apiEcosystemID, gAuth, gPrivate, txname, form)
	return err
}

func CutErr(err error) string {
	out := err.Error()
	if off := strings.IndexByte(out, '('); off != -1 {
		out = out[:off]
	}
	return strings.TrimSpace(out)
}

// add "txHash string"
func PostTxResult(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, name string, form getter) (id int64, txHash string, msg string, err error) {
	var contract getContractResult
	if err = sendGet(apiAddress, gAuth, "contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]interface{})
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		//
		case "array":
			var v interface{}
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]interface{}
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

	/*data, _, err := tx.NewTransaction(tx.SmartContract{
		Header: tx.Header{
			ID:          int(contract.ID),
			Time:        time.Now().Unix(),
			EcosystemID: 1,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   consts.NETWORK_ID,
		},
		Params: params,
	}, privateKey)*/
	data, _, err := NewTransaction(SmartContract{
		Header: Header{
			ID:   int(contract.ID),
			Time: time.Now().Unix(),
			//EcosystemID: 1,
			EcosystemID: apiEcosystemID,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.NetworkID,
			PrivateFor:  ApiPrivateFor,
		},
		Params: params,
	}, privateKey)
	if err != nil {

		//
		//return 0, "", err
		return 0, "", "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart(apiAddress, gAuth, "sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	// add
	txHash = ret.Hashes["data"]

	if len(form.Get("nowait")) > 0 {
		return
	}

	id, err = waitTx(apiAddress, gAuth, ret.Hashes["data"])
	if id != 0 && err != nil {
		msg = err.Error()
		err = nil
	}

	return
}

//add "txHash string"
func ChainPostTxResult(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, name string, form getter) (id int64, txHash string, msg string, err error) {
	var contract getContractResult
	if err = sendGet(apiAddress, gAuth, "contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]interface{})
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
			//
		case "array":
			var v interface{}
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]interface{}
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

	/*data, _, err := tx.NewTransaction(tx.SmartContract{
		Header: tx.Header{
			ID:          int(contract.ID),
			Time:        time.Now().Unix(),
			EcosystemID: 1,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   consts.NETWORK_ID,
		},
		Params: params,
	}, privateKey)*/
	data, _, err := NewTransaction(SmartContract{
		Header: Header{
			ID:   int(contract.ID),
			Time: time.Now().Unix(),
			//EcosystemID: 1,
			EcosystemID: apiEcosystemID,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.NetworkID,
			PrivateFor:  ApiPrivateFor,
		},
		Params: params,
	}, privateKey)
	if err != nil {

		// add
		//return 0, "", err
		return 0, "", "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart(apiAddress, gAuth, "sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	// add
	txHash = ret.Hashes["data"]

	if len(form.Get("nowait")) > 0 {
		return
	}

	//id, err = waitTx(ret.Hashes["data"])
	//if id != 0 && err != nil {
	//	msg = err.Error()
	//	err = nil
	//}

	return
}

// add "txHash string"
func VDEPostTxResult(apiAddress string, apiEcosystemID int64, gAuth string, gPrivate string, name string, form getter) (id int64, txHash string, msg string, err error) {
	var contract getContractResult
	if err = sendGet(apiAddress, gAuth, "contract/"+name, nil, &contract); err != nil {
		return
	}

	params := make(map[string]interface{})
	for _, field := range contract.Fields {
		name := field.Name
		value := form.Get(name)

		if len(value) == 0 {
			continue
		}

		switch field.Type {
		case "bool":
			params[name], err = strconv.ParseBool(value)
		case "int":
			params[name], err = strconv.ParseInt(value, 10, 64)
		case "float":
			params[name], err = strconv.ParseFloat(value, 64)
		case "array":
			var v interface{}
			err = json.Unmarshal([]byte(value), &v)
			params[name] = v
		case "map":
			var v map[string]interface{}
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

	/*data, _, err := tx.NewTransaction(tx.SmartContract{
		Header: tx.Header{
			ID:          int(contract.ID),
			Time:        time.Now().Unix(),
			EcosystemID: 1,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   consts.NETWORK_ID,
		},
		Params: params,
	}, privateKey)*/
	data, _, err := NewTransaction(SmartContract{
		Header: Header{
			ID:   int(contract.ID),
			Time: time.Now().Unix(),
			//EcosystemID: 1,
			EcosystemID: apiEcosystemID,
			KeyID:       crypto.Address(publicKey),
			NetworkID:   conf.Config.NetworkID,
			PrivateFor:  ApiPrivateFor,
		},
		Params: params,
	}, privateKey)
	if err != nil {

		// add
		//return 0, "", err
		return 0, "", "", err
	}

	ret := &sendTxResult{}
	err = sendMultipart(apiAddress, gAuth, "sendTx", map[string][]byte{
		"data": data,
	}, &ret)
	if err != nil {
		return
	}

	// add
	txHash = ret.Hashes["data"]

	if len(form.Get("nowait")) > 0 {
		return
	}

	//id, err = waitTx(ret.Hashes["data"])
	//if id != 0 && err != nil {
	//	msg = err.Error()
	//	err = nil
	//}

	return
}

func WaitTx(apiAddress string, gAuth string, hash string) (int64, error) {
	data, err := json.Marshal(&txstatusRequest{
		Hashes: []string{hash},
	})
	if err != nil {
		return 0, err
	}

	for i := 0; i < 15; i++ {
		var multiRet multiTxStatusResult
		err := sendPost(apiAddress, gAuth, `txstatus`, &url.Values{
			"data": {string(data)},
		}, &multiRet)
		if err != nil {
			return 0, err
		}

		ret := multiRet.Results[hash]

		if len(ret.BlockID) > 0 {
			return converter.StrToInt64(ret.BlockID), fmt.Errorf(ret.Result)
		}
		if ret.Message != nil {
			errtext, err := json.Marshal(ret.Message)
			if err != nil {
				return 0, err
			}
			return 0, errors.New(string(errtext))
		}
		time.Sleep(time.Second)
	}
	return 0, fmt.Errorf(`TxStatus timeout`)
}

//0312
func VDEWaitTx(apiAddress string, gAuth string, hash string) (int64, error) {
	data, err := json.Marshal(&txstatusRequest{
		Hashes: []string{hash},
	})
	if err != nil {
		return 0, err
	}

	for i := 0; i < 15; i++ {
		var multiRet multiTxStatusResult
		err := sendPost(apiAddress, gAuth, `txstatus`, &url.Values{
			"data": {string(data)},
		}, &multiRet)
		if err != nil {
			return 0, err
		}

		ret := multiRet.Results[hash]

		if len(ret.BlockID) > 0 {
			return converter.StrToInt64(ret.BlockID), fmt.Errorf(ret.Result)
		}
		if ret.Message != nil {
			errtext, err := json.Marshal(ret.Message)
			if err != nil {
				return 0, err
			}
			return 0, errors.New(string(errtext))
		}
		time.Sleep(time.Second)
	}
	//return 0, fmt.Errorf(`TxStatus timeout`)
	return -1, fmt.Errorf(`TxStatus timeout`)
}

func RandName(prefix string) string {
	return fmt.Sprintf(`%s%d`, prefix, time.Now().Unix())
}
