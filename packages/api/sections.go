/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
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
	// The sections of no role are everyone's, the others are their roles' only
	q := sqldb.GetDB(nil).Table(table).
		Where("ecosystem = ? AND status > 0", client.EcosystemID).
		Where("(roles_access IS NULL OR roles_access = '[]'::jsonb OR roles_access @> ?::jsonb)", fmt.Sprintf("[%d]", client.RoleID)).
		Order("id ASC")

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
