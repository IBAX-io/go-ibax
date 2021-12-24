/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	log "github.com/sirupsen/logrus"
)

const publicKeyLength = 64

var (
	errHonorNodeInvalidValues       = errors.New("invalid values of the honor_nodes parameter")
	errHonorNodeDuplicatePublicKey  = errors.New("duplicate publicKey values of the honor_nodes parameter")
	errHonorNodeDuplicateAPIAddress = errors.New("duplicate api address values of the honor_nodes parameter")
	errHonorNodeDuplicateTCPAddress = errors.New("duplicate tcp address values of the honor_nodes parameter")
)

type honorNodeJSON struct {
	TCPAddress string      `json:"tcp_address"`
	APIAddress string      `json:"api_address"`
	PublicKey  string      `json:"public_key"`
	UnbanTime  json.Number `json:"unban_time,er"`
	Stopped    bool        `json:"stopped"`
}

// HonorNode is storing honor node data
type HonorNode struct {
	TCPAddress string
	APIAddress string
	PublicKey  []byte
	UnbanTime  time.Time
	Stopped    bool
}

// UnmarshalJSON is custom json unmarshaller
func (fn *HonorNode) UnmarshalJSON(b []byte) (err error) {
	data := honorNodeJSON{}
	if err = json.Unmarshal(b, &data); err != nil {
		log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err, "value": string(b)}).Error("Unmarshalling honor nodes to json")
		return err
	}

	fn.TCPAddress = data.TCPAddress
	fn.APIAddress = data.APIAddress
	fn.Stopped = data.Stopped
	if fn.PublicKey, err = crypto.HexToPub(data.PublicKey); err != nil {
		log.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": data.PublicKey}).Error("converting honor nodes public key from hex")
		return err
	}
	fn.UnbanTime = time.Unix(converter.StrToInt64(data.UnbanTime.String()), 0)

	if err = fn.Validate(); err != nil {
		return err
	}

	return nil
}

func (fn *HonorNode) MarshalJSON() ([]byte, error) {
	jfn := honorNodeJSON{
		TCPAddress: fn.TCPAddress,
		APIAddress: fn.APIAddress,
		PublicKey:  crypto.PubToHex(fn.PublicKey),
		UnbanTime:  json.Number(strconv.FormatInt(fn.UnbanTime.Unix(), 10)),
	}

	data, err := json.Marshal(jfn)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("Marshalling honor nodes to json")
		return nil, err
	}

	return data, nil
}

// ValidateURL returns error if the URL is invalid
func validateURL(rawurl string) error {
	u, err := url.ParseRequestURI(rawurl)
	if err != nil {
		return err
	}

	if len(u.Scheme) == 0 {
		return fmt.Errorf("invalid scheme: %s", rawurl)
	}

	if len(u.Host) == 0 {
		return fmt.Errorf("invalid host: %s", rawurl)
	}

	return nil
}

// Validate checks values
func (fn *HonorNode) Validate() error {
	if len(fn.PublicKey) !=
		publicKeyLength || len(fn.TCPAddress) == 0 {
		return errHonorNodeInvalidValues
	}

	if err := validateURL(fn.APIAddress); err != nil {
		return err
	}

	return nil
}

func DuplicateHonorNode(fn []*HonorNode) error {
	n := len(fn)
	var dup error
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j > 0 && reflect.DeepEqual(fn[i].PublicKey, fn[j].PublicKey) {
				dup = errHonorNodeDuplicatePublicKey
				break
			}
			if j > 0 && reflect.DeepEqual(fn[i].APIAddress, fn[j].APIAddress) {
				dup = errHonorNodeDuplicateAPIAddress
				break
			}
			if j > 0 && reflect.DeepEqual(fn[i].TCPAddress, fn[j].TCPAddress) {
				dup = errHonorNodeDuplicateTCPAddress
				break
			}
		}
		if vali := fn[i].Validate(); vali != nil {
			return vali
		}
	}
	return dup
}
