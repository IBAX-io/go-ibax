/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package api

import (
	"encoding/json"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type roleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type notifyInfo struct {
	RoleID string `json:"role_id"`
	Count  int64  `json:"count"`
}

type keyInfoResult struct {
	Account    string              `json:"account"`
	Ecosystems []*keyEcosystemInfo `json:"ecosystems"`
}

type keyEcosystemInfo struct {
	Ecosystem     string       `json:"ecosystem"`
	Name          string       `json:"name"`
	Digits        int64        `json:"digits"`
	Roles         []roleInfo   `json:"roles,omitempty"`
	Notifications []notifyInfo `json:"notifications,omitempty"`
}

func (m Mode) getKeyInfoHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	keysList := make([]*keyEcosystemInfo, 0)
	keyID := converter.StringToAddress(params["wallet"])
	if keyID == 0 {
		errorResponse(w, errInvalidWallet.Errorf(params["wallet"]))
		return
	}

	ids, names, err := m.EcosystemGetter.GetEcosystemLookup()
	if err != nil {
		errorResponse(w, err)
		return
	}

	var (
		account string
		found   bool
	)

	for i, ecosystemID := range ids {
		key := &sqldb.Key{}
		key.SetTablePrefix(ecosystemID)
		found, err = key.Get(nil, keyID)
		if err != nil {
			errorResponse(w, err)
			return
		}
		if !found {
			continue
		}

		// TODO: delete after switching to another account storage scheme
		if len(account) == 0 {
			account = key.AccountID
		}
		eco := sqldb.Ecosystem{}
		_, err = eco.Get(nil, ecosystemID)
		if err != nil {
			errorResponse(w, err)
			return
		}
		keyRes := &keyEcosystemInfo{
			Ecosystem: converter.Int64ToStr(ecosystemID),
			Name:      names[i],
			Digits:    eco.Digits,
		}
		ra := &sqldb.RolesParticipants{}
		roles, err := ra.SetTablePrefix(ecosystemID).GetActiveMemberRoles(key.AccountID)
		if err != nil {
			errorResponse(w, err)
			return
		}
		for _, r := range roles {
			var role roleInfo
			if err := json.Unmarshal([]byte(r.Role), &role); err != nil {
				logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling role")
				errorResponse(w, err)
				return
			}
			keyRes.Roles = append(keyRes.Roles, role)
		}
		keyRes.Notifications, err = m.getNotifications(ecosystemID, key)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting notifications")
			errorResponse(w, err)
			return
		}

		keysList = append(keysList, keyRes)
	}

	// in test mode, registration is open in the first ecosystem
	if len(keysList) == 0 {
		account = converter.AddressToString(keyID)
		notify := make([]notifyInfo, 0)
		notify = append(notify, notifyInfo{})
		keysList = append(keysList, &keyEcosystemInfo{
			Ecosystem:     converter.Int64ToStr(ids[0]),
			Name:          names[0],
			Notifications: notify,
		})
	}

	jsonResponse(w, &keyInfoResult{
		Account:    account,
		Ecosystems: keysList,
	})
}

func (m Mode) getNotifications(ecosystemID int64, key *sqldb.Key) ([]notifyInfo, error) {
	notif, err := sqldb.GetNotificationsCount(ecosystemID, []string{key.AccountID})
	if err != nil {
		return nil, err
	}

	list := make([]notifyInfo, 0)
	for _, n := range notif {
		if n.RecipientID != key.ID {
			continue
		}

		list = append(list, notifyInfo{
			RoleID: converter.Int64ToStr(n.RoleID),
			Count:  n.Count,
		})
	}
	return list, nil
}
