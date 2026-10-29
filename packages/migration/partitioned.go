/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"fmt"
	"strings"
)

// PartitionBlocks is the number of blocks a partition of a table partitioned by block covers
const PartitionBlocks = 10000

const (
	// PartsSchema holds the partitions of the active tables, apart from the tables the contracts
	// create, so that no name of a partition is the name of a table of an ecosystem
	PartsSchema = "parts"
	// RetiredSchema holds the partitions taken out of the active tables at the last multiple of
	// PartitionBlocks, until the next one: the rollback of that block puts them back
	RetiredSchema = "retired"
	// ArchiveSchema holds, on an archive node, the partitions out of the window, as partitions of
	// tables of the names of the active ones
	ArchiveSchema = "archive"
)

// GenesisPartition is the partition of block_chain that holds block 1, which never leaves it
const GenesisPartition = "block_chain_genesis"

// UnspentPartition is the partition of spent_info that holds the unspent outputs, of spent_block 0,
// which never leaves it
const UnspentPartition = "spent_info_unspent"

// PartitionedTable is a table partitioned by ranges of PartitionBlocks blocks of its Key column
type PartitionedTable struct {
	Name string
	Key  string
	// ID is whether the table has an id column that GetNextID gives
	ID bool
	// ByTime is whether its partitions leave by the time of their transactions (MaxTxBack)
	// instead of by the history_blocks window
	ByTime bool
	// columns and constraints, and the indexes on the partitioned table
	columns string
	indexes []string
	// fixed is the CREATE TABLE of the partition that never leaves, if any, and more the statements
	// after the partitions are made
	fixed string
	more  []string
}

// PartitionedTables are the tables whose rows leave the active tables by ranges of blocks
var PartitionedTables = []PartitionedTable{
	{
		Name: "block_chain", Key: "id",
		columns: `"id" bigint NOT NULL DEFAULT '0',
		"hash" bytea NOT NULL DEFAULT '',
		"rollbacks_hash" bytea NOT NULL DEFAULT '',
		"data" bytea NOT NULL DEFAULT '',
		"ecosystem_id" integer NOT NULL DEFAULT '0',
		"key_id" bigint NOT NULL DEFAULT '0',
		"node_position" bigint NOT NULL DEFAULT '0',
		"time" bigint NOT NULL DEFAULT '0',
		"tx" integer NOT NULL DEFAULT '0',
		"consensus_mode" integer NOT NULL DEFAULT '1',
		"candidate_nodes" bytea NOT NULL DEFAULT '',
		PRIMARY KEY ("id")`,
		indexes: []string{`"hash"`, `"node_position", "time"`},
		fixed: fmt.Sprintf(`CREATE TABLE "%s"."%s" PARTITION OF "block_chain" FOR VALUES FROM (MINVALUE) TO (2);`,
			PartsSchema, GenesisPartition),
	},
	{
		Name: "confirmations", Key: "block_id",
		columns: `"block_id" bigint NOT NULL DEFAULT '0',
		"good" integer NOT NULL DEFAULT '0',
		"bad" integer NOT NULL DEFAULT '0',
		"time" bigint NOT NULL DEFAULT '0',
		PRIMARY KEY ("block_id")`,
	},
	{
		Name: "bft_certificates", Key: "height",
		columns: `"height" bigint NOT NULL DEFAULT '0',
		"round" bigint NOT NULL DEFAULT '0',
		"hash" bytea NOT NULL DEFAULT '',
		"data" bytea NOT NULL DEFAULT '',
		PRIMARY KEY ("height")`,
	},
	{
		Name: "rollback_tx", Key: "block_id", ID: true,
		columns: `"id" bigint NOT NULL DEFAULT '0',
		"block_id" bigint NOT NULL DEFAULT '0',
		"tx_hash" bytea NOT NULL DEFAULT '',
		"table_name" varchar(255) NOT NULL DEFAULT '',
		"table_id" varchar(255) NOT NULL DEFAULT '',
		"data_hash" bytea NOT NULL DEFAULT '',
		"data" text NOT NULL DEFAULT '',
		PRIMARY KEY ("block_id", "id")`,
		indexes: []string{`"id"`, `"tx_hash"`, `"table_name", "table_id", "block_id"`},
	},
	{
		Name: "1_history", Key: "block_id", ID: true,
		columns: `"id" bigint NOT NULL DEFAULT '0',
		"sender_id" bigint NOT NULL DEFAULT '0',
		"recipient_id" bigint NOT NULL DEFAULT '0',
		"sender_balance" decimal(30) NOT NULL DEFAULT '0',
		"recipient_balance" decimal(30) NOT NULL DEFAULT '0',
		"amount" decimal(30) NOT NULL DEFAULT '0',
		"value_detail" jsonb,
		"comment" text NOT NULL DEFAULT '',
		"status" bigint NOT NULL DEFAULT '0',
		"block_id" bigint NOT NULL DEFAULT '0',
		"txhash" bytea NOT NULL DEFAULT '',
		"created_at" bigint NOT NULL DEFAULT '0',
		"ecosystem" bigint NOT NULL DEFAULT '1',
		"type" bigint NOT NULL DEFAULT '1',
		PRIMARY KEY ("block_id", "id")`,
		indexes: []string{`"id"`, `"txhash"`, `"ecosystem", "sender_id"`, `"ecosystem", "recipient_id"`},
	},
	{
		Name: "log_transactions", Key: "block", ByTime: true,
		columns: `"hash" bytea NOT NULL DEFAULT '',
		"block" bigint NOT NULL DEFAULT '0',
		"timestamp" bigint NOT NULL DEFAULT '0',
		"contract_name" varchar(255) NOT NULL DEFAULT '',
		"address" bigint NOT NULL DEFAULT '0',
		"ecosystem_id" bigint NOT NULL DEFAULT '0',
		"status" bigint NOT NULL DEFAULT '0',
		PRIMARY KEY ("block", "hash")`,
		indexes: []string{`"hash"`},
	},
	// an output stays in the unspent partition until a block spends it, and moves then to the range
	// of that block, which leaves with the ranges of the other tables of its blocks
	{
		Name: "spent_info", Key: "spent_block",
		columns: `"input_tx_hash" bytea,
		"input_index" integer,
		"output_tx_hash" bytea NOT NULL,
		"output_index" integer NOT NULL,
		"output_key_id" bigint NOT NULL,
		"output_value" decimal(30) NOT NULL DEFAULT '0' CHECK ("output_value" >= 0),
		"ecosystem" bigint NOT NULL DEFAULT '1',
		"block_id" bigint NOT NULL,
		"type" bigint NOT NULL,
		"spent_block" bigint NOT NULL DEFAULT '0',
		CONSTRAINT "spent_info_spent" CHECK (("input_tx_hash" IS NULL) = ("spent_block" = 0)),
		PRIMARY KEY ("block_id", "output_tx_hash", "output_key_id", "output_index", "spent_block")`,
		indexes: []string{`"ecosystem", "output_key_id", "block_id"`},
		fixed: fmt.Sprintf(`CREATE TABLE "%s"."%s" PARTITION OF "spent_info" FOR VALUES FROM (0) TO (1);`,
			PartsSchema, UnspentPartition),
		more: []string{
			fmt.Sprintf(`CREATE INDEX ON "%s"."%s" ("output_key_id", "ecosystem");`, PartsSchema, UnspentPartition),
			`CREATE INDEX ON "spent_info" ("spent_block", "input_tx_hash") WHERE "spent_block" > 0;`,
		},
	},
}

// PartitionedTableOf is the partitioned table of the name
func PartitionedTableOf(name string) (PartitionedTable, bool) {
	for _, t := range PartitionedTables {
		if t.Name == name {
			return t, true
		}
	}
	return PartitionedTable{}, false
}

// PartitionName names the partition of range k of the table
func PartitionName(table string, k int64) string {
	return fmt.Sprintf("%s_p%d", table, k)
}

// PartitionBounds is the FOR VALUES clause of range k of the table: range 0 of block_chain starts
// after block 1, which GenesisPartition holds, and range 0 of spent_info after spent_block 0, which
// UnspentPartition holds
func PartitionBounds(table string, k int64) string {
	from := fmt.Sprint(k * PartitionBlocks)
	if k == 0 {
		switch table {
		case "block_chain":
			from = "2"
		case "spent_info":
			from = "1"
		default:
			from = "MINVALUE"
		}
	}
	return fmt.Sprintf("FROM (%s) TO (%d)", from, (k+1)*PartitionBlocks)
}

// CreatePartitionSQL creates the partition of range k of the table, unless it exists
func CreatePartitionSQL(table string, k int64) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s"."%s" PARTITION OF "%s" FOR VALUES %s;`,
		PartsSchema, PartitionName(table, k), table, PartitionBounds(table, k))
}

// sql drops and creates the table, its indexes, the partition that never leaves and the partitions
// of ranges 0 and 1
func (t PartitionedTable) sql() string {
	var b strings.Builder
	fmt.Fprintf(&b, "DROP TABLE IF EXISTS \"%[1]s\" CASCADE;\nCREATE TABLE \"%[1]s\" (\n\t\t%[2]s\n\t) PARTITION BY RANGE (\"%[3]s\");\n",
		t.Name, t.columns, t.Key)
	for _, cols := range t.indexes {
		fmt.Fprintf(&b, "CREATE INDEX ON \"%s\" (%s);\n", t.Name, cols)
	}
	if t.fixed != "" {
		b.WriteString(t.fixed + "\n")
	}
	b.WriteString(CreatePartitionSQL(t.Name, 0) + "\n")
	b.WriteString(CreatePartitionSQL(t.Name, 1) + "\n")
	for _, sql := range t.more {
		b.WriteString(sql + "\n")
	}
	return b.String()
}

// HistoryTableSQL creates 1_history, with the tables of the first ecosystem
func HistoryTableSQL() string {
	t, _ := PartitionedTableOf("1_history")
	return t.sql()
}

// retentionSQL creates the schemas of the partitions, the partitioned tables but 1_history and
// the tables of the retention of rows, and of what outlives it: the count of the successful
// transactions of every ecosystem and the hashes of the StopNetwork transactions
func retentionSQL() string {
	var b strings.Builder
	for _, s := range []string{PartsSchema, RetiredSchema, ArchiveSchema} {
		fmt.Fprintf(&b, "DROP SCHEMA IF EXISTS \"%[1]s\" CASCADE;\nCREATE SCHEMA \"%[1]s\";\n", s)
	}
	for _, t := range PartitionedTables {
		if t.Name != "1_history" {
			b.WriteString(t.sql())
		}
	}
	b.WriteString(`
	DROP TABLE IF EXISTS "retired_partitions";
	CREATE TABLE "retired_partitions" (
		"boundary" bigint NOT NULL,
		"table_name" varchar(255) NOT NULL,
		"k" bigint NOT NULL,
		PRIMARY KEY ("boundary", "table_name", "k")
	);
	DROP TABLE IF EXISTS "log_transaction_ranges";
	CREATE TABLE "log_transaction_ranges" (
		"k" bigint NOT NULL PRIMARY KEY,
		"last_time" bigint NOT NULL
	);
	DROP TABLE IF EXISTS "partition_id_floors";
	CREATE TABLE "partition_id_floors" (
		"table_name" varchar(255) NOT NULL PRIMARY KEY,
		"max_id" bigint NOT NULL
	);
	DROP TABLE IF EXISTS "retention_state";
	CREATE TABLE "retention_state" (
		"name" varchar(255) NOT NULL PRIMARY KEY,
		"value" bigint NOT NULL
	);
	DROP TABLE IF EXISTS "log_transaction_counts";
	CREATE TABLE "log_transaction_counts" (
		"ecosystem_id" bigint NOT NULL PRIMARY KEY,
		"count" bigint NOT NULL
	);
	DROP TABLE IF EXISTS "stop_network_txs";
	CREATE TABLE "stop_network_txs" (
		"hash" bytea NOT NULL PRIMARY KEY,
		"block" bigint NOT NULL
	);
`)
	return b.String()
}
