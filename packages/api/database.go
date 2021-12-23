package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

type tableInfoForm struct {
	Order      string `schema:"order"`
	Where      string `schema:"where"`
	Limit      int    `schema:"limit"`
	Page       int    `json:"page"`
	Table_name string `json:"table_name,omitempty"`
}
type columnsInfo struct {
	Table_name string `json:"table_name,omitempty"`
}
type rowsInfo struct {
	tableInfoForm
}

func (f *tableInfoForm) Validate(r *http.Request) error {
	if f.Page < 1 || f.Limit < 1 {
		return errors.New("limit or page is unvalid")
	}
	return nil
}
func (f *columnsInfo) Validate(r *http.Request) error {
	if f.Table_name == "" {
		return errors.New("tablename is null")
	}
	return nil
}
func (f *rowsInfo) Validate(r *http.Request) error {
	if f.Page < 1 || f.Limit < 1 {
		return errors.New("limit or page is unvalid")
	}
	return nil
}

func getOpenDatabaseInfoHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	sqlQuery := "SELECT current_user,CURRENT_CATALOG,VERSION (),pg_size_pretty(pg_database_size (CURRENT_CATALOG)),pg_postmaster_start_time() FROM pg_user LIMIT 1"
	rows, err := sqldb.GetDB(nil).Raw(sqlQuery).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("GetDatabaseInfo rows failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	list, err := sqldb.GetResult(rows)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("GetDatabaseInfo result failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	if list == nil {
		jsonResponse(w, nil)
		return
	}
	jsonResponse(w, list[0])
}

func getOpenTablesInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &tableInfoForm{}
	result := &listResult{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	logger := getLogger(r)
	if form.Order == "" {
		form.Order = "tablename asc"
	}
	q := sqldb.GetDB(nil)
	ns := "%" + form.Table_name + "%"
	if err := q.Table("pg_tables").Where("schemaname='public' and tablename like ?", ns).Count(&result.Count).Error; err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getOpenTables row from table")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	query := fmt.Sprintf("SELECT tablename from pg_tables where schemaname ='public' and tablename like %s order by %s offset %d limit %d", "'%"+form.Table_name+"%'", form.Order, (form.Page-1)*form.Limit, form.Limit)
	rows, err := q.Raw(query).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": query}).Error("getOpenTables rows from tablesInfo")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.List, err = sqldb.GetResult(rows)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getOpenTables getResult")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	jsonResponse(w, result)
}

func getOpenColumnsInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &columnsInfo{}
	result := &listResult{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	logger := getLogger(r)
	//client:=getClient(r)
	order := "ordinal_position ASC"

	sqlQuery := fmt.Sprintf("SELECT column_name,data_type,column_default FROM information_schema.columns WHERE table_name='%s' ORDER BY %s", form.Table_name, order)
	rows, err := sqldb.GetDB(nil).Raw(sqlQuery).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": sqlQuery}).Error("get colums info failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.List, err = sqldb.GetResult(rows)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get open Cloumns result info failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.Count = int64(len(result.List))
	jsonResponse(w, result)
}

func getOpenRowsInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &rowsInfo{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	logger := getLogger(r)

	result, err := GetRowsInfo(form.Table_name, form.Order, form.Page, form.Limit, form.Where)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get rows info failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	jsonResponse(w, result)
}
func GetRowsInfo(tableName, order string, page, limit int, where string) (*listResult, error) {
	result := &listResult{}
	num, err := sqldb.GetNodeRows(tableName)
	if err != nil {
		return result, err
	}
	defaultorder := "asc"
	if order != "" {
		if strings.Contains(order, "desc") || strings.Contains(order, "DESC") {
			defaultorder = "desc"
		}
	}
	var primaryOrder = make(map[string]string)
	primaryOrder["confirmations"] = "block_id " + defaultorder
	primaryOrder["info_block"] = "block_id " + defaultorder
	primaryOrder["install"] = "progress " + defaultorder
	primaryOrder["log_transactions"] = "hash " + defaultorder
	primaryOrder["queue_blocks"] = "hash " + defaultorder
	primaryOrder["queue_tx"] = "hash " + defaultorder
	primaryOrder["stop_daemons"] = "stop_time " + defaultorder
	primaryOrder["transactions"] = "hash " + defaultorder
	//primaryOrder["transactions_attempts"] = "hash " + defaultorder
	primaryOrder["transactions_status"] = "hash " + defaultorder
	execOrder := order
	if v, ok := primaryOrder[tableName]; ok {
		execOrder = v
	}
	if execOrder == "" {
		err = fmt.Errorf("order is null")
		return nil, err
	}

	result.Count = num
	var sqlQuest string
	if where == "" {
		sqlQuest = fmt.Sprintf(`select * from "%s" order by %s offset %d limit %d`, tableName, execOrder, (page-1)*limit, limit)
	} else {
		sqlQuest = fmt.Sprintf(`select * from "%s" where %s order by %s offset %d limit %d`, tableName, where, execOrder, (page-1)*limit, limit)
	}
	rows, err := sqldb.GetDB(nil).Raw(sqlQuest).Rows()
	if err != nil {
		return result, fmt.Errorf("getRows raw err:%s in query %s", err, sqlQuest)
	}

	result.List, err = sqldb.GetRowsInfo(rows, sqlQuest)
	if err != nil {
		return nil, err
	}
	return result, nil
}
