/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/golang-jwt/jwt/v4"
)

type AuthStatusResponse struct {
	IsActive  bool  `json:"active"`
	ExpiresAt int64 `json:"exp,omitempty"`
}

type Auth struct {
	Mode
}

func authRequire(r *http.Request) *Error {
	client := getClient(r)
	if client != nil && client.KeyID != 0 {
		return nil
	}

	logger := getLogger(r)
	logger.WithFields(log.Fields{"type": consts.EmptyObject}).Debug("wallet is empty")
	return UnauthorizedError()
}

type authApi struct {
	mode Mode
}

func NewAuthApi(mode Mode) *authApi {
	a := &authApi{
		mode: mode,
	}
	return a
}

func (a *authApi) GetAuthStatus(ctx RequestContext) (*AuthStatusResponse, *Error) {
	result := new(AuthStatusResponse)

	r := ctx.HTTPRequest()
	token := getToken(r)
	if token == nil {
		return result, nil
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return result, nil
	}

	result.IsActive = true
	result.ExpiresAt = claims.ExpiresAt.Unix()
	return result, nil
}

type GetUIDResult struct {
	UID         string `json:"uid,omitempty"`
	Token       string `json:"token,omitempty"`
	Expire      string `json:"expire,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	Address     string `json:"address,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	Cryptoer    string `json:"cryptoer"`
	Hasher      string `json:"hasher"`
}

func (a *authApi) GetUid(ctx RequestContext) (*GetUIDResult, *Error) {
	const jwtUIDExpire = time.Second * 5

	result := new(GetUIDResult)
	result.NetworkID = converter.Int64ToStr(conf.Config.LocalConf.NetworkID)
	r := ctx.HTTPRequest()
	token := getToken(r)
	result.Cryptoer, result.Hasher = conf.Config.CryptoSettings.Cryptoer, conf.Config.CryptoSettings.Hasher
	if token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok && len(claims.KeyID) > 0 {
			result.EcosystemID = claims.EcosystemID
			result.Expire = claims.ExpiresAt.Sub(time.Now()).String()
			result.KeyID = claims.KeyID
			result.Address = converter.AddressToString(converter.StrToInt64(claims.KeyID))
			return result, nil
		}
	}

	result.UID = converter.Int64ToStr(rand.New(rand.NewSource(time.Now().Unix())).Int63())
	claims := JWTClaims{
		UID:         result.UID,
		EcosystemID: "1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(jwtUIDExpire)},
		},
	}

	var err error
	if result.Token, err = generateJWTToken(claims); err != nil {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.JWTError, "error": err}).Error("generating jwt token")
		return nil, DefaultError(err.Error())
	}
	return result, nil
}

// Special word used by frontend to sign UID generated by /getuid API command, sign is performed for contcatenated word and UID
func nonceSalt() string {
	return fmt.Sprintf("LOGIN%d", conf.Config.LocalConf.NetworkID)
}

type loginForm struct {
	EcosystemID int64          `json:"ecosystem_id"`
	Expire      int64          `json:"expire"`
	PublicKey   publicKeyValue `json:"public_key"`
	KeyID       string         `json:"key_id"`
	Signature   hexValue       `json:"signature"`
	RoleID      int64          `json:"role_id"`
	IsMobile    bool           `json:"is_mobile"`
}

type publicKeyValue struct {
	hexValue
}

func (pk *publicKeyValue) UnmarshalText(v []byte) (err error) {
	pk.value, err = hex.DecodeString(string(v))
	pk.value = crypto.CutPub(pk.value)
	return
}

func (f *loginForm) Validate(r *http.Request) error {
	if f == nil {
		return errors.New(paramsEmpty)
	}
	if f.Expire == 0 {
		f.Expire = int64(jwtExpire)
	}

	return nil
}

type LoginResult struct {
	Token       string        `json:"token,omitempty"`
	EcosystemID string        `json:"ecosystem_id,omitempty"`
	KeyID       string        `json:"key_id,omitempty"`
	Account     string        `json:"account,omitempty"`
	NotifyKey   string        `json:"notify_key,omitempty"`
	IsNode      bool          `json:"isnode"`
	IsOwner     bool          `json:"isowner"`
	IsCLB       bool          `json:"clb"`
	Timestamp   string        `json:"timestamp,omitempty"`
	Roles       []rolesResult `json:"roles,omitempty"`
}

type rolesResult struct {
	RoleID   int64  `json:"role_id"`
	RoleName string `json:"role_name"`
}

func (a authApi) Login(ctx RequestContext, form *loginForm) (*LoginResult, *Error) {
	var (
		publicKey           []byte
		wallet, founder, fm int64
		isExistPub          bool
		spfounder, spfm     sqldb.StateParameter
	)
	r := ctx.HTTPRequest()
	uid, err := getUID(r)
	if err != nil {
		return nil, UnUnknownUIDError()
	}
	if err := form.Validate(r); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	client := getClient(r)
	logger := getLogger(r)

	if form.EcosystemID > 0 {
		client.EcosystemID = form.EcosystemID
	} else if client.EcosystemID == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Warning("state is empty, using 1 as a state")
		client.EcosystemID = 1
	}

	if len(form.KeyID) > 0 {
		wallet = converter.StringToAddress(form.KeyID)
	} else if len(form.PublicKey.Bytes()) > 0 {
		wallet = crypto.Address(form.PublicKey.Bytes())
	}

	account := &sqldb.Key{}
	account.SetTablePrefix(client.EcosystemID)
	isAccount, err := account.Get(nil, wallet)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	spfm.SetTablePrefix(converter.Int64ToStr(client.EcosystemID))
	if ok, err := spfm.Get(nil, "free_membership"); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting free_membership parameter")
		return nil, DefaultError(err.Error())
	} else if ok {
		fm = converter.StrToInt64(spfm.Value)
	}
	publicKey = account.PublicKey
	isExistPub = len(publicKey) == 0

	isCan := func(a, e bool) bool {
		return !a || (a && e)
	}
	if isCan(isAccount, isExistPub) {
		if !(fm == 1 || client.EcosystemID == 1) {
			return nil, DefaultError(fmt.Sprintf("The ecosystem (%d) is not open and cannot be registered address", client.EcosystemID))
		}
	}

	if isAccount && !isExistPub {
		if account.Deleted == 1 {
			return nil, DefaultError("The key is deleted")
		}
	} else {
		if !allowCreateUser(client) {
			return nil, DefaultError("Key has not been found")
		}
		if isCan(isAccount, isExistPub) {
			publicKey = form.PublicKey.Bytes()
			if len(publicKey) == 0 {
				logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("public key is empty")
				return nil, DefaultError("Public key is undefined")
			}

			nodePrivateKey := syspar.GetNodePrivKey()

			contract := smart.GetContract("NewUser", 1)
			sc := types.SmartTransaction{
				Header: &types.Header{
					ID:          int(contract.Info().ID),
					EcosystemID: 1,
					Time:        time.Now().Unix(),
					KeyID:       conf.Config.KeyID,
					NetworkID:   conf.Config.LocalConf.NetworkID,
				},
				Params: map[string]any{
					"NewPubkey": hex.EncodeToString(publicKey),
					"Ecosystem": client.EcosystemID,
				},
			}

			stp := &transaction.SmartTransactionParser{
				SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
			}
			txData, err := stp.BinMarshalWithPrivate(&sc, nodePrivateKey, true)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.ContractError, "err": err}).Error("Building transaction")
				return nil, DefaultError(err.Error())
			}

			if err := a.mode.ContractRunner.RunContract(txData, stp.Hash, sc.KeyID, stp.Timestamp, logger); err != nil {
				return nil, DefaultError(err.Error())
			}

			if !conf.Config.IsSupportingCLB() {
				gt := 3 * syspar.GetMaxBlockGenerationTime()
				l := &sqldb.LogTransaction{}
				for i := 0; i < 2; i++ {
					found, err := l.GetByHash(nil, stp.Hash)
					if err != nil {
						return nil, DefaultError(err.Error())
					}
					if found {
						if l.Status != 0 {
							return nil, DefaultError("encountered some problems when login account")
						} else {
							_, _ = account.Get(nil, wallet)
							break
						}
					}
					time.Sleep(time.Duration(gt) * time.Millisecond)
				}

				if l.Block == 0 {
					return nil, DefaultError("The block packing in progress, please wait")
				}
			}

		} else {
			logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("public key is empty, and state is not default")
			return nil, DefaultError(fmt.Sprintf("%d is not a membership of ecosystem %d", wallet, client.EcosystemID))
		}
	}

	if len(publicKey) == 0 {
		if client.EcosystemID > 1 {
			logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("public key is empty, and state is not default")
			return nil, DefaultError(fmt.Sprintf("%d is not a membership of ecosystem %d", wallet, client.EcosystemID))
		}

		if len(form.PublicKey.Bytes()) == 0 {
			logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("public key is empty")
			return nil, DefaultError("Public key is undefined")
		}
	}

	if form.RoleID != 0 && client.RoleID == 0 {
		checkedRole, err := checkRoleFromParam(form.RoleID, client.EcosystemID, account.AccountID)
		if err != nil {
			return nil, DefaultError(err.Error())
		}

		if checkedRole != form.RoleID {
			return nil, DefaultError("Access denied")
		}

		client.RoleID = checkedRole
	}

	verify, err := crypto.Verify(publicKey, []byte(nonceSalt()+uid), form.Signature.Bytes())
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.CryptoError, "pubkey": publicKey, "uid": uid, "signature": form.Signature.Bytes()}).Info("checking signature")
		return nil, DefaultError(err.Error())
	}

	if !verify {
		logger.WithFields(log.Fields{"type": consts.InvalidObject, "pubkey": publicKey, "uid": uid, "signature": form.Signature.Bytes()}).Error("incorrect signature")
		return nil, DefaultError("Signature is incorrect")
	}

	spfounder.SetTablePrefix(converter.Int64ToStr(client.EcosystemID))
	if ok, err := spfounder.Get(nil, "founder_account"); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting founder_account parameter")
		return nil, DefaultError(err.Error())
	} else if ok {
		founder = converter.StrToInt64(spfounder.Value)
	}

	result := &LoginResult{
		Account:     account.AccountID,
		EcosystemID: converter.Int64ToStr(client.EcosystemID),
		KeyID:       converter.Int64ToStr(wallet),
		IsOwner:     founder == wallet,
		IsNode:      conf.Config.KeyID == wallet,
		IsCLB:       conf.Config.IsSupportingCLB(),
	}

	claims := JWTClaims{
		KeyID:       result.KeyID,
		AccountID:   account.AccountID,
		EcosystemID: result.EcosystemID,
		IsMobile:    form.IsMobile,
		RoleID:      converter.Int64ToStr(form.RoleID),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(time.Second * time.Duration(form.Expire))},
		},
	}

	result.Token, err = generateJWTToken(claims)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JWTError, "error": err}).Error("generating jwt token")
		return nil, DefaultError(err.Error())
	}

	result.NotifyKey, result.Timestamp, err = publisher.GetJWTCent(wallet, form.Expire)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	ra := &sqldb.RolesParticipants{}
	roles, err := ra.SetTablePrefix(client.EcosystemID).GetActiveMemberRoles(account.AccountID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting roles")
		return nil, DefaultError(err.Error())
	}

	for _, r := range roles {
		var res map[string]string
		if err := json.Unmarshal([]byte(r.Role), &res); err != nil {
			log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling role")
			return nil, DefaultError(err.Error())
		}

		result.Roles = append(result.Roles, rolesResult{
			RoleID:   converter.StrToInt64(res["id"]),
			RoleName: res["name"],
		})
	}

	return result, nil
}

func getUID(r *http.Request) (string, error) {
	var uid string

	token := getToken(r)
	if token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok {
			uid = claims.UID
		}
	} else if len(uid) == 0 {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Warning("UID is empty")
		return "", errors.New("unknown uid")
	}

	return uid, nil
}

func checkRoleFromParam(role, ecosystemID int64, account string) (int64, error) {
	if role > 0 {
		ok, err := sqldb.MemberHasRole(nil, role, ecosystemID, account)
		if err != nil {
			log.WithFields(log.Fields{
				"type":      consts.DBError,
				"account":   account,
				"role":      role,
				"ecosystem": ecosystemID}).Error("check role")

			return 0, err
		}

		if !ok {
			log.WithFields(log.Fields{
				"type":      consts.NotFound,
				"account":   account,
				"role":      role,
				"ecosystem": ecosystemID,
			}).Error("member hasn't role")

			return 0, nil
		}
	}
	return role, nil
}

func allowCreateUser(c *UserClient) bool {
	if conf.Config.IsSupportingCLB() {
		return true
	}

	return true
}
