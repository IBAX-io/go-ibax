/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"bytes"

	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	xl "github.com/360EntSecGroup-Skylar/excelize"
	log "github.com/sirupsen/logrus"
)

// GetDataFromXLSX returns json by parameters range
func GetDataFromXLSX(sc *SmartContract, binaryID, startLine, linesCount, sheetNum int64) (data []any, err error) {
	book, err := excelBookFromStoredBinary(sc, binaryID)
	if err != nil || book == nil {
		return nil, err
	}

	sheetName := book.GetSheetName(int(sheetNum))
	rows := book.GetRows(sheetName)
	endLine := startLine + linesCount
	if endLine > int64(len(rows)) {
		endLine = int64(len(rows))
	}
	processedRows := []any{}
	for ; startLine < endLine; startLine++ {
		var row []any
		for _, item := range rows[startLine] {
			row = append(row, item)
		}
		processedRows = append(processedRows, row)
	}
	return processedRows, nil
}

// GetRowsCountXLSX returns count of rows from excel file
func GetRowsCountXLSX(sc *SmartContract, binaryID, sheetNum int64) (int64, error) {
	book, err := excelBookFromStoredBinary(sc, binaryID)
	if err != nil {
		return -1, err
	}

	sheetName := book.GetSheetName(int(sheetNum))
	rows := book.GetRows(sheetName)
	return int64(len(rows)), nil
}

func excelBookFromStoredBinary(sc *SmartContract, binaryID int64) (*xl.File, error) {
	bin := &sqldb.Binary{}
	bin.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID))
	found, err := bin.GetByID(binaryID)
	if err != nil {
		return nil, err
	}

	if !found {
		log.WithFields(log.Fields{"binary_id": binaryID}).Error("binary_id not found")
		return nil, nil
	}

	return xl.OpenReader(bytes.NewReader(bin.Data))
}
