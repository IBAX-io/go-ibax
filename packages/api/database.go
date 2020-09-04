package api

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type tableInfoForm struct {
	listForm

	Order   string `schema:"order"`
	InWhere string `schema:"where"`
}
type columnsInfo struct {
	listForm
	Order   string `schema:"order"`
	InWhere string `schema:"where"`
	Name    string `schema:"name"`
}
type rowsInfo struct {
	columnsInfo
}

func (f *tableInfoForm) Validate(r *http.Request) error {
	if err := f.listForm.Validate(r); err != nil {
		return err
	}
	return nil
}
func (f *columnsInfo) Validate(r *http.Request) error {
	if err := f.listForm.Validate(r); err != nil {
		return err
	}
	return nil
}
func (f *rowsInfo) Validate(r *http.Request) error {
	if err := f.listForm.Validate(r); err != nil {
		return err
	}
	return nil
}

func getOpenDatabaseInfoHandler(w http.ResponseWriter, r *http.Request) {
	result := &listResult{}
	logger := getLogger(r)
	sqlQuery := "SELECT current_user,CURRENT_CATALOG,VERSION (),pg_size_pretty(pg_database_size (CURRENT_CATALOG)),pg_postmaster_start_time() FROM pg_user LIMIT 1"
	rows, err := model.GetDB(nil).Raw(sqlQuery).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("GetDatabaseInfo rows failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.List, err = model.GetResult(rows)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("GetDatabaseInfo result failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.Count = 1
	jsonResponse(w, result)
}

func getOpenTablesInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &tableInfoForm{}
	result := &listResult{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	logger := getLogger(r)
	if form.Limit < 1 || form.Offset < 0 {
		err := fmt.Errorf("limit less than 1 recv:%d or offset is negative recv:%d", form.Limit, form.Offset)
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	order := "tablename asc"
	q := model.GetDB(nil)
	if err := q.Raw("SELECT count(*) FROM pg_tables WHERE schemaname='public'").Take(&result.Count).Error; err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getOpenTables row from table")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	query := fmt.Sprintf("select tablename from pg_tables where schemaname ='public' order by %s offset %d limit %d", order, form.Offset, form.Limit)
	rows, err := q.Raw(query).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": query}).Error("getOpenTables rows from tablesInfo")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.List, err = model.GetResult(rows)
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
	if len(form.Name) == 0 {
		err := fmt.Errorf("tableName is null")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	sqlQuery := fmt.Sprintf("SELECT column_name,data_type,column_default FROM information_schema.columns WHERE table_name='%s' ORDER BY %s", form.Name, order)
	rows, err := model.GetDB(nil).Raw(sqlQuery).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": sqlQuery}).Error("get colums info failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result.List, err = model.GetResult(rows)
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
	if form.Limit < 1 || form.Offset < 0 {
		err := fmt.Errorf("limit less than 1 recv:%d or offset is negative recv:%d", form.Limit, form.Offset)
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	result, err := GetRowsInfo(form.Name, form.Order, form.Offset, form.Limit, form.InWhere)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get rows info failed")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	jsonResponse(w, result)
}
func GetRowsInfo(tableName, order string, offset, limit int, where string) (*listResult, error) {
	result := &listResult{}
	num, err := model.GetNodeRows(tableName)
	if err != nil {
		return result, err
	}
	var primaryOrder = make(map[string]string)
	primaryOrder["confirmations"] = "block_id desc"
	primaryOrder["info_block"] = "hash asc"
	primaryOrder["install"] = "progress asc"
	primaryOrder["log_transactions"] = "hash asc"
	primaryOrder["queue_blocks"] = "hash asc"
	primaryOrder["queue_tx"] = "hash asc"
	primaryOrder["stop_daemons"] = "stop_time asc"
	primaryOrder["transactions"] = "hash asc"
	primaryOrder["transactions_attempts"] = "hash asc"
	primaryOrder["transactions_status"] = "hash asc"
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
		sqlQuest = fmt.Sprintf(`select * from "%s" order by %s offset %d limit %d`, tableName, execOrder, offset, limit)
	} else {
		sqlQuest = fmt.Sprintf(`select * from "%s" where %s order by %s offset %d limit %d`, tableName, where, execOrder, offset, limit)
	}
	rows, err := model.GetDB(nil).Raw(sqlQuest).Rows()
	if err != nil {
		return result, fmt.Errorf("getRows raw err:%s in query %s", err, sqlQuest)
