/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func getAvatarHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	account := params["account"]
	ecosystemID := converter.StrToInt64(params["ecosystem"])

	member := &sqldb.Member{}
	member.SetTablePrefix(converter.Int64ToStr(ecosystemID))

	found, err := member.Get(account)
	if err != nil {
		logger.WithFields(log.Fields{
			"type":      consts.DBError,
			"error":     err,
			"ecosystem": ecosystemID,
			"account":   account,
		}).Error("getting member")
		errorResponse(w, err)
		return
	}

	if !found {
		errorResponse(w, errNotFoundRecord)
		return
	}

	if member.ImageID == nil {
		errorResponse(w, errNotFoundRecord)
		return
	}

	bin := &sqldb.Binary{}
	bin.SetTablePrefix(converter.Int64ToStr(ecosystemID))
	found, err = bin.GetByID(*member.ImageID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "image_id": *member.ImageID}).Errorf("on getting binary by id")
		errorResponse(w, err)
		return
	}

	if !found {
		errorResponse(w, errNotFound)
		return
	}

	if len(bin.Data) == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject, "error": err, "image_id": *member.ImageID}).Errorf("on check avatar size")
		errorResponse(w, errNotFound)
		return
	}

	w.Header().Set("Content-Type", bin.MimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(bin.Data)))
	if _, err := w.Write(bin.Data); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("unable to write image")
	}
}

func getMemberHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	account := params["account"]
	ecosystemID := converter.StrToInt64(params["ecosystem"])

	member := &sqldb.Member{}
	member.SetTablePrefix(converter.Int64ToStr(ecosystemID))

	_, err := member.Get(account)
	if err != nil {
		logger.WithFields(log.Fields{
			"type":      consts.DBError,
			"error":     err,
			"ecosystem": ecosystemID,
			"account":   account,
		}).Error("getting member")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, member)
}
