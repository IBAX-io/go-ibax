/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package login is how a client gets a session of the node's APIs, REST and JSON-RPC alike: it
// asks for a challenge (UID), signs "LOGIN" + network id + uid with its key, and logs in with the
// signature. Docs: docs/api/login.md.
package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
)

// UIDResult is the answer of getuid: a new challenge, or what the session token sent stands for
type UIDResult struct {
	UID         string `json:"uid,omitempty"`
	Token       string `json:"token,omitempty"`
	Expire      string `json:"expire,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	Address     string `json:"address,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	Cryptoer    string `json:"cryptoer"`
	Hasher      string `json:"hasher"`
	Fips        bool   `json:"fips"` // the node runs in FIPS 140-3 mode
	// AccountAlgorithms are the algorithms account keys may have and their days; Cryptoer is the
	// algorithm of the node keys, an account signs with the algorithm of its own key
	AccountAlgorithms syspar.AccountAlgorithmSet `json:"account_algorithms"`
}

// Request is a login: the claims of the token sent with it (nil without one) and its form
type Request struct {
	Token     *Claims
	Ecosystem int64
	Expire    int64 // seconds, 0 for SessionLifetime
	PublicKey []byte
	KeyID     string
	Signature []byte
	RoleID    int64
	// Guest signs in as the guest account (consts.GuestKey), without a signature: its key is
	// public, a signature with it proves nothing. Only Ecosystem and Expire count then.
	Guest bool
}

// Result is the session a login opens
type Result struct {
	Token        string `json:"token,omitempty"`
	EcosystemID  string `json:"ecosystem_id,omitempty"`
	KeyID        string `json:"key_id,omitempty"`
	Account      string `json:"account,omitempty"`
	NotifyKey    string `json:"notify_key,omitempty"`
	IsNode       bool   `json:"isnode"`
	IsOwner      bool   `json:"isowner"`
	IsChildChain bool   `json:"clb"` // JSON tag kept for API compatibility
	Timestamp    string `json:"timestamp,omitempty"`
	Roles        []Role `json:"roles,omitempty"`
}

// Role is a role of the account in the ecosystem
type Role struct {
	RoleID   int64  `json:"role_id"`
	RoleName string `json:"role_name"`
}

// Salt is what a login signature covers before the uid
func Salt() string {
	return fmt.Sprintf("LOGIN%d", conf.Config.LocalConf.NetworkID)
}

// UID answers getuid: with a session token, what it stands for; otherwise a new challenge
func UID(token *Claims) (*UIDResult, error) {
	result := &UIDResult{
		NetworkID: converter.Int64ToStr(conf.Config.LocalConf.NetworkID),
		Cryptoer:  conf.Config.CryptoSettings.Cryptoer,
		Hasher:    conf.Config.CryptoSettings.Hasher,
		Fips:      crypto.FIPSMode(),

		AccountAlgorithms: syspar.GetAccountAlgorithms(),
	}
	if token != nil && len(token.KeyID) > 0 {
		result.EcosystemID = token.EcosystemID
		result.Expire = time.Until(token.ExpiresAt.Time).String()
		result.KeyID = token.KeyID
		result.Address = converter.AddressToString(converter.StrToInt64(token.KeyID))
		return result, nil
	}
	var err error
	result.UID, result.Token, err = challenge(time.Now())
	return result, err
}

// Login checks a login and opens its session. The challenge is used up whatever the outcome; the
// signature is checked before anything else is read, and nothing is ever written.
func Login(req Request, logger *log.Entry) (*Result, error) {
	if req.Token == nil || len(req.Token.UID) == 0 || !pending.consume(req.Token.UID, time.Now()) {
		return nil, ErrUnknownUID
	}
	if req.Expire < 0 || req.Expire > SessionLifetime {
		return nil, ErrExpire
	}
	if req.Expire == 0 {
		req.Expire = SessionLifetime
	}
	if req.Ecosystem <= 0 {
		req.Ecosystem = 1
	}

	var (
		wallet  int64
		account *sqldb.Key
		err     error
	)
	if req.Guest {
		wallet, req.RoleID = converter.StrToInt64(consts.GuestKey), 0
		account = &sqldb.Key{}
		found, err := account.SetTablePrefix(req.Ecosystem).Get(nil, wallet)
		if err != nil {
			return nil, err
		}
		if !found || account.Deleted == 1 {
			return nil, ErrKeyNotFound
		}
	} else if wallet, account, err = signIn(req, logger); err != nil {
		return nil, err
	}

	if req.RoleID != 0 {
		member, err := sqldb.MemberHasRole(nil, req.RoleID, req.Ecosystem, account.AccountID)
		if err != nil {
			return nil, err
		}
		if !member {
			return nil, ErrCheckRole
		}
	}
	return session(req, wallet, account)
}

// signIn checks the signature of a login, and finds the account it signs in to. The key that
// signs is the registered one; a key the ecosystem does not know signs with the key it sends,
// which must be of an algorithm still registered: it is told to register with @1NewUser.
func signIn(req Request, logger *log.Entry) (int64, *sqldb.Key, error) {
	var wallet int64
	switch {
	case len(req.PublicKey) > 0:
		key, err := crypto.ParseAccountKey(req.PublicKey)
		if err != nil {
			return 0, nil, ErrKeyAlgorithm(err.Error())
		}
		wallet = key.Address()
		if len(req.KeyID) > 0 && converter.StringToAddress(req.KeyID) != wallet {
			return 0, nil, ErrDiffKey
		}
	case len(req.KeyID) > 0:
		wallet = converter.StringToAddress(req.KeyID)
	default:
		return 0, nil, ErrEmptyPublic
	}

	account := &sqldb.Key{}
	found, err := account.SetTablePrefix(req.Ecosystem).Get(nil, wallet)
	if err != nil {
		return 0, nil, err
	}
	var registered []byte
	if found {
		registered = account.PublicKey
	}
	// A login is not in a block: the days of account_algorithms are compared with the clock
	key, err := syspar.GetAccountAlgorithms().Signer(registered, req.PublicKey, wallet, time.Now().Unix())
	switch {
	case errors.Is(err, syspar.ErrNoAccountKey):
		return 0, nil, ErrEmptyPublic
	case errors.Is(err, syspar.ErrAccountKeyID):
		return 0, nil, ErrDiffKey
	case err != nil:
		return 0, nil, ErrKeyAlgorithm(err.Error())
	}

	ok, err := key.Verify([]byte(Salt()+req.Token.UID), req.Signature)
	if err != nil || !ok {
		logger.WithFields(log.Fields{"type": consts.InvalidObject, "key_id": wallet, "error": err}).Info("incorrect login signature")
		if err != nil {
			return 0, nil, ErrSignature(err.Error())
		}
		return 0, nil, ErrSignature("Signature is incorrect")
	}
	// The session is of the key that signed, whatever was asked for
	if key.Address() != wallet {
		return 0, nil, ErrDiffKey
	}

	if len(registered) > 0 {
		if account.Deleted == 1 {
			return 0, nil, ErrDeletedKey
		}
		return wallet, account, nil
	}
	if req.Ecosystem != 1 {
		open, err := freeMembership(req.Ecosystem)
		if err != nil {
			return 0, nil, err
		}
		if !open {
			return 0, nil, ErrEcoNotOpen(req.Ecosystem)
		}
	}
	return 0, nil, ErrNewUser
}

func freeMembership(ecosystem int64) (bool, error) {
	param := &sqldb.StateParameter{}
	param.SetTablePrefix(converter.Int64ToStr(ecosystem))
	found, err := param.Get(nil, "free_membership")
	return found && converter.StrToInt64(param.Value) == 1, err
}

func session(req Request, wallet int64, account *sqldb.Key) (*Result, error) {
	var founder int64
	param := &sqldb.StateParameter{}
	param.SetTablePrefix(converter.Int64ToStr(req.Ecosystem))
	if found, err := param.Get(nil, "founder_account"); err != nil {
		return nil, err
	} else if found {
		founder = converter.StrToInt64(param.Value)
	}

	result := &Result{
		Account:      account.AccountID,
		EcosystemID:  converter.Int64ToStr(req.Ecosystem),
		KeyID:        converter.Int64ToStr(wallet),
		IsOwner:      founder == wallet,
		IsNode:       conf.Config.KeyID == wallet,
		IsChildChain: conf.Config.IsSupportingChildChain(),
	}
	var err error
	if result.Token, err = Sign(Claims{
		KeyID:       result.KeyID,
		AccountID:   account.AccountID,
		EcosystemID: result.EcosystemID,
		RoleID:      converter.Int64ToStr(req.RoleID),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Second * time.Duration(req.Expire))),
		},
	}); err != nil {
		return nil, err
	}
	if result.NotifyKey, err = publisher.ConnectionToken(account.AccountID, req.Expire); err != nil {
		return nil, err
	}
	result.Timestamp = converter.Int64ToStr(time.Now().Unix())

	roles, err := (&sqldb.RolesParticipants{}).SetTablePrefix(req.Ecosystem).GetActiveMemberRoles(account.AccountID)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		var role map[string]string
		if err := json.Unmarshal([]byte(r.Role), &role); err != nil {
			return nil, err
		}
		result.Roles = append(result.Roles, Role{RoleID: converter.StrToInt64(role["id"]), RoleName: role["name"]})
	}
	return result, nil
}

// Discard uses up the challenge of a login refused before Login could check it
func Discard(token *Claims) {
	if token != nil && len(token.UID) > 0 {
		pending.consume(token.UID, time.Now())
	}
}
