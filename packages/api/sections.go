/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

const defaultSectionsLimit = 100

type sectionsForm struct {
	paginatorForm
	Lang string `schema:"lang"`
}

func (f *sectionsForm) Validate(r *http.Request) error {
	if err := f.paginatorForm.Validate(r); err != nil {
		return err
	}

	if len(f.Lang) == 0 {
		f.Lang = r.Header.Get("Accept-Language")
	}

	return nil
}

func getSectionsHandler(w http.ResponseWriter, r *http.Request) {
	form := &sectionsForm{}
	form.defaultLimit = defaultSectionsLimit
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	client := getClient(r)
	logger := getLogger(r)

	table := "1_sections"
	q := sqldb.GetDB(nil).Table(table).Where("ecosystem = ? AND status > 0", client.EcosystemID).Order("id ASC")

	result := new(listResult)
	err := q.Count(&result.Count).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting table records count")
		errorResponse(w, errTableNotFound.Errorf(table))
		return
	}

	rows, err := q.Offset(form.Offset).Limit(form.Limit).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
		errorResponse(w, err)
		return
	}

	result.List, err = sqldb.GetResult(rows)
	if err != nil {
		errorResponse(w, err)
		return
	}

	var sections []map[string]string
	for _, item := range result.List {
		var roles []int64
		if err := json.Unmarshal([]byte(item["roles_access"]), &roles); err != nil {
			errorResponse(w, err)
			return
		}
		if len(roles) > 0 {
			var added bool
			for _, v := range roles {
				if v == client.RoleID {
					added = true
					break
				}
			}
			if !added {
				continue
			}
		}

		if item["status"] == consts.StatusMainPage {
			roles := &sqldb.Role{}
			roles.SetTablePrefix(1)
			role, err := roles.Get(nil, client.RoleID)

			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Debug("Getting role by id")
				errorResponse(w, err)
				return
			}
			if role == true && roles.DefaultPage != "" {
				item["default_page"] = roles.DefaultPage
			}
		}

		item["title"] = language.LangMacro(item["title"], int(client.EcosystemID), form.Lang)
		sections = append(sections, item)
	}
	result.List = sections

	jsonResponse(w, result)
}
