/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPresentStatuses(t *testing.T) {
	assert.Empty(t, presentStatuses(nil))
	assert.Empty(t, presentStatuses([]*TransactionStatus{nil, nil}))
	a, b := &TransactionStatus{Hash: []byte{1}}, &TransactionStatus{Hash: []byte{2}}
	assert.Equal(t, []*TransactionStatus{a, b}, presentStatuses([]*TransactionStatus{nil, a, nil, b}))
}

func TestUpsertBlockStatusStatement(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost", PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	assert.NoError(t, err)
	statuses := []*TransactionStatus{
		{Hash: []byte{1}, Time: 10, WalletID: -5, BlockID: 7, Error: "it's done"},
		nil,
		{Hash: []byte{2}, Time: 11, WalletID: 6, BlockID: 7, Error: `{"type":"panic"}`, Penalty: 1},
	}
	rows := presentStatuses(statuses)
	result := db.Clauses(upsertBlockStatusClause).Create(&rows)
	assert.NoError(t, result.Error)
	stmt := result.Statement
	assert.Equal(t,
		`INSERT INTO "transactions_status" ("hash","time","type","wallet_id","block_id","error","penalty") VALUES ($1,$2,$3,$4,$5,$6,$7),($8,$9,$10,$11,$12,$13,$14) ON CONFLICT ("hash") DO UPDATE SET "block_id"="excluded"."block_id","error"="excluded"."error","penalty"="excluded"."penalty"`,
		stmt.SQL.String())
	assert.Equal(t, []any{
		[]byte{1}, int64(10), byte(0), int64(-5), int64(7), "it's done", int64(0),
		[]byte{2}, int64(11), byte(0), int64(6), int64(7), `{"type":"panic"}`, int64(1),
	}, stmt.Vars)
}
