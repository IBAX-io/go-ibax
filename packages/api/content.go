/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/template"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type contentResult struct {
	Menu       string          `json:"menu,omitempty"`
	MenuTree   json.RawMessage `json:"menutree,omitempty"`
	Title      string          `json:"title,omitempty"`
	Tree       json.RawMessage `json:"tree"`
	NodesCount int64           `json:"nodesCount,omitempty"`
}

type hashResult struct {
	Hash string `json:"hash"`
}

const (
	strTrue = `true`
	strOne  = `1`
)

var errEmptyTemplate = errors.New("Empty template")

func initVars(r *http.Request) *map[string]string {
	client := getClient(r)
	r.ParseMultipartForm(multipartBuf)

	vars := make(map[string]string)
	for name := range r.Form {
		vars[name] = r.FormValue(name)
	}
	vars["_full"] = "0"
	vars["current_time"] = fmt.Sprintf("%d", time.Now().Unix())
	vars["guest_key"] = consts.GuestKey
	vars["guest_account"] = consts.GuestAddress
	vars["black_hole_key"] = strconv.FormatInt(converter.HoleAddrMap[converter.BlackHoleAddr].K, 10)
	vars["black_hole_account"] = converter.HoleAddrMap[converter.BlackHoleAddr].S
	vars["white_hole_key"] = strconv.FormatInt(converter.HoleAddrMap[converter.WhiteHoleAddr].K, 10)
	vars["white_hole_account"] = converter.HoleAddrMap[converter.WhiteHoleAddr].S
	if client.KeyID != 0 {
		vars["ecosystem_id"] = converter.Int64ToStr(client.EcosystemID)
		vars["key_id"] = converter.Int64ToStr(client.KeyID)
		vars["account_id"] = client.AccountID
		vars["isMobile"] = isMobileValue(client.IsMobile)
		vars["role_id"] = converter.Int64ToStr(client.RoleID)
		vars["ecosystem_name"] = client.EcosystemName
	} else {
		vars["ecosystem_id"] = vars["ecosystem"]
		delete(vars, "ecosystem")
		if len(vars["keyID"]) > 0 {
			vars["key_id"] = vars["keyID"]
			vars["account_id"] = converter.AddressToString(converter.StrToInt64(vars["keyID"]))
		} else {
			vars["key_id"] = "0"
			vars["account_id"] = ""
		}
		if len(vars["roleID"]) > 0 {
			vars["role_id"] = vars["roleID"]
		} else {
			vars["role_id"] = "0"
		}
		if len(vars["isMobile"]) == 0 {
			vars["isMobile"] = "0"
		}
		if len(vars["ecosystem_id"]) != 0 {
			ecosystems := sqldb.Ecosystem{}
			if found, _ := ecosystems.Get(nil, converter.StrToInt64(vars["ecosystem_id"])); found {
				vars["ecosystem_name"] = ecosystems.Name
			}
		}
	}
	if _, ok := vars["lang"]; !ok {
		vars["lang"] = r.Header.Get("Accept-Language")
	}

	return &vars
}

func isMobileValue(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func parseEcosystem(in string) (string, string) {
	ecosystem, name := converter.ParseName(in)
	if ecosystem == 0 {
		return ``, name
	}
	return converter.Int64ToStr(ecosystem), name
}

func pageValue(r *http.Request) (*sqldb.Page, string, error) {
	params := mux.Vars(r)
	logger := getLogger(r)
	client := getClient(r)

	var ecosystem string
	page := &sqldb.Page{}
	name := params["name"]
	if strings.HasPrefix(name, `@`) {
		ecosystem, name = parseEcosystem(name)
		if len(name) == 0 {
			logger.WithFields(log.Fields{
				"type":  consts.NotFound,
				"value": params["name"],
			}).Debug("page not found")
			return nil, ``, errNotFound
		}
	} else {
		ecosystem = client.Prefix()
	}
	page.SetTablePrefix(ecosystem)
	found, err := page.Get(name)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting page")
		return nil, ``, err
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("page not found")
		return nil, ``, errNotFound
	}
	return page, ecosystem, nil
}

func getPage(r *http.Request) (result *contentResult, err error) {
	page, _, err := pageValue(r)
	if err != nil {
		return nil, err
	}

	logger := getLogger(r)

	client := getClient(r)
	menu := &sqldb.Menu{}
	menu.SetTablePrefix(client.Prefix())
	_, err = menu.Get(page.Menu)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting page menu")
		return nil, errServer
	}
	var wg sync.WaitGroup
	var timeout bool
	wg.Add(2)
	success := make(chan bool, 1)
	go func() {
		defer wg.Done()

		vars := initVars(r)
		(*vars)["app_id"] = converter.Int64ToStr(page.AppID)

		ret := template.Template2JSON(page.Value, &timeout, vars)
		if timeout {
			return
		}
		retmenu := template.Template2JSON(menu.Value, &timeout, vars)
		if timeout {
			return
		}
		result = &contentResult{
			Tree:       ret,
			Menu:       page.Menu,
			MenuTree:   retmenu,
			NodesCount: page.ValidateCount,
		}
		success <- true
	}()
	go func() {
		defer wg.Done()
		if conf.Config.LocalConf.MaxPageGenerationTime == 0 {
			return
		}
		select {
		case <-time.After(time.Duration(conf.Config.LocalConf.MaxPageGenerationTime) * time.Millisecond):
			timeout = true
		case <-success:
		}
	}()
	wg.Wait()
	close(success)

	if timeout {
		logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error(page.Name + " is a heavy page")
		return nil, errHeavyPage
	}

	return result, nil
}

func getPageHandler(w http.ResponseWriter, r *http.Request) {
	result, err := getPage(r)
	if err != nil {
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func getPageHashHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	params := mux.Vars(r)

	if ecosystem := r.FormValue("ecosystem"); len(ecosystem) > 0 &&
		!strings.HasPrefix(params["name"], "@") {
		params["name"] = "@" + ecosystem + params["name"]
	}
	result, err := getPage(r)
	if err != nil {
		errorResponse(w, err)
		return
	}

	out, err := json.Marshal(result)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("getting string for hash")
		errorResponse(w, errServer)
		return
	}

	jsonResponse(w, &hashResult{Hash: hex.EncodeToString(crypto.Hash(out))})
}

func getMenuHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	client := getClient(r)
	logger := getLogger(r)

	var ecosystem string
	menu := &sqldb.Menu{}
	name := params["name"]
	if strings.HasPrefix(name, `@`) {
		ecosystem, name = parseEcosystem(name)
		if len(name) == 0 {
			logger.WithFields(log.Fields{
				"type":  consts.NotFound,
				"value": params["name"],
			}).Debug("page not found")
			errorResponse(w, errNotFound)
			return
		}
	} else {
		ecosystem = client.Prefix()
	}

	menu.SetTablePrefix(ecosystem)
	found, err := menu.Get(name)

	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting menu")
		errorResponse(w, err)
		return
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("menu not found")
		errorResponse(w, errNotFound)
		return
	}
	var timeout bool
	ret := template.Template2JSON(menu.Value, &timeout, initVars(r))
	jsonResponse(w, &contentResult{Tree: ret, Title: menu.Title})
}

type jsonContentForm struct {
	Template string `schema:"template"`
	Source   bool   `schema:"source"`
}

func (f *jsonContentForm) Validate(r *http.Request) error {
	if len(f.Template) == 0 {
		return errEmptyTemplate
	}
	return nil
}

func jsonContentHandler(w http.ResponseWriter, r *http.Request) {
	form := &jsonContentForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	var timeout bool
	vars := initVars(r)

	if form.Source {
		(*vars)["_full"] = strOne
	}

	ret := template.Template2JSON(form.Template, &timeout, vars)
	jsonResponse(w, &contentResult{Tree: ret})
}

func getSourceHandler(w http.ResponseWriter, r *http.Request) {
	page, _, err := pageValue(r)
	if err != nil {
		errorResponse(w, err)
		return
	}
	var timeout bool
	vars := initVars(r)
	(*vars)["_full"] = strOne
	ret := template.Template2JSON(page.Value, &timeout, vars)

	jsonResponse(w, &contentResult{Tree: ret})
}

func getPageValidatorsCountHandler(w http.ResponseWriter, r *http.Request) {
	page, _, err := pageValue(r)
	if err != nil {
		errorResponse(w, err)
		return
	}

	res := map[string]int64{"validate_count": page.ValidateCount}
	jsonResponse(w, &res)
}
