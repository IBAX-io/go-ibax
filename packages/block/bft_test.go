/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// The restore after a trial reloads the platform parameters and the column types a block changed
// in its rolled back database transaction, as UpdatePlatformParam and the table functions change
// them: the chain of TestTrialAndCommitOnChain may have no block that does. It runs with the same
// IBAX_TEST_CHAIN_CONFIG and IBAX_TEST_CHAIN_DB and changes nothing in the database.
func TestMemStateRestoreReloads(t *testing.T) {
	config, name := os.Getenv("IBAX_TEST_CHAIN_CONFIG"), os.Getenv("IBAX_TEST_CHAIN_DB")
	if config == "" || name == "" {
		t.Skip("IBAX_TEST_CHAIN_CONFIG and IBAX_TEST_CHAIN_DB name no node and no copy of its database")
	}
	require.NoError(t, conf.LoadConfig(config))
	conf.Config.DB.Name = name
	require.NoError(t, sqldb.GormInit(conf.Config.DB))
	log.SetLevel(log.FatalLevel)
	require.NoError(t, syspar.SysUpdate(nil))
	require.NoError(t, syspar.SysTableColType(nil))

	params, cols := syspar.GetSysParCache(), syspar.GetTableColType()
	mem := takeMemState()
	dbTx, err := sqldb.StartTransaction()
	require.NoError(t, err)
	require.NoError(t, dbTx.ExecSql(`UPDATE "1_platform_parameters" SET value = '7' WHERE name = 'gap_between_blocks'`))
	require.NoError(t, syspar.SysUpdate(dbTx))
	require.NoError(t, dbTx.ExecSql(`CREATE TABLE "1_bft_trial_cols" (id bigint, data bytea)`))
	require.NoError(t, syspar.SysTableColType(dbTx))
	require.Equal(t, "7", syspar.SysString("gap_between_blocks"))
	require.True(t, syspar.IsByteColumn("1_bft_trial_cols", "data"))
	require.NoError(t, dbTx.Rollback())

	require.NoError(t, mem.restore())
	require.Equal(t, params, syspar.GetSysParCache())
	require.Equal(t, cols, syspar.GetTableColType())
	require.False(t, syspar.IsByteColumn("1_bft_trial_cols", "data"))
}
