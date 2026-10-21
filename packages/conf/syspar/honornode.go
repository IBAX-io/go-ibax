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
	if len(fn.PublicKey) != crypto.NodePublicKeySize() || len(fn.TCPAddress) == 0 {
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

// MinBftValidators is the least number of validators of a chain of the BFT consensus: with N
// validators it bears f = (N-1)/3 faulty ones, and 4 is the least N with f = 1
const MinBftValidators = 4

// CheckBftValidators checks the honor_nodes of the first block of a chain of the BFT consensus:
// valid nodes, no two alike, and at least MinBftValidators of them not stopped
func CheckBftValidators(nodes []*HonorNode) error {
	if err := DuplicateHonorNode(nodes); err != nil {
		return err
	}
	if active := activeBftValidators(nodes, nil); active < MinBftValidators {
		return fmt.Errorf("%d validators are not stopped: a chain of the BFT consensus has at least %d", active, MinBftValidators)
	}
	return nil
}

// CheckBftValidatorsChange checks a new value of honor_nodes on a chain of the BFT consensus
// against the value it replaces (design 4.1): valid nodes, no two alike, and a vote that removes
// or stops a validator of the set fails if the set then has fewer than MinBftValidators. The set
// is the nodes neither stopped nor out by an evidence, which out reports. A vote that adds
// validators, or removes a node an evidence put out, runs whatever the size of the set: only an
// evidence takes the set below MinBftValidators, and only a vote takes it back.
func CheckBftValidatorsChange(prev, next []*HonorNode, out func(publicKey []byte) bool) error {
	if err := DuplicateHonorNode(next); err != nil {
		return err
	}
	before, after := activeBftValidators(prev, out), activeBftValidators(next, out)
	if after < MinBftValidators && after < before {
		return fmt.Errorf("the set goes from %d to %d validators: a vote takes no validator out of a set below %d", before, after, MinBftValidators)
	}
	return nil
}

// activeBftValidators is the nodes neither stopped nor out
func activeBftValidators(nodes []*HonorNode, out func(publicKey []byte) bool) int {
	var active int
	for _, n := range nodes {
		if !n.Stopped && (out == nil || !out(n.PublicKey)) {
			active++
		}
	}
	return active
}
