/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hotQueries are the queries a node runs per transaction or per block, as the bench profiles them
// (pg_stat_statements), with values in place of the parameters, and the indexes each must read by:
// an index of a partition is matched by its name with the range as p\d+
var hotQueries = []struct {
	sql     string
	indexes []string
}{
	{`SELECT * FROM "1_keys" WHERE id = 1 and ecosystem = 1 ORDER BY "1_keys"."id" LIMIT 1`, []string{`1_keys_pkey`}},
	{`SELECT id, amount, "ecosystem" FROM "1_keys" WHERE "ecosystem" = 1 and "account" = '0000-0000-0000-0000-0000' and "deleted" = 0`,
		[]string{`1_keys_ecosystem_account`}},
	{`UPDATE "1_keys" SET amount = amount + 1 WHERE "ecosystem" = 1 and "id" = 1`, []string{`1_keys_pkey`}},
	{`SELECT * FROM "1_ecosystems" WHERE id = 1`, []string{`1_ecosystems_pkey`}},
	{`SELECT * FROM "1_parameters" WHERE ecosystem = 1 and name = 'utxo_fee'`, []string{`1_parameters_ecosystem_name_idx`}},
	{`SELECT * FROM "1_members" WHERE "ecosystem" = 1 and "account" = '0000-0000-0000-0000-0000'`,
		[]string{`1_members_account_ecosystem_idx`}},
	// an ecosystem has tens of applications
	{`SELECT * FROM "1_applications" WHERE "ecosystem" = 1 and "name" = 'Basic' and "deleted" = 0`,
		[]string{`1_applications_ecosystem_idx`}},
	{`SELECT * FROM "transactions_status" WHERE hash = '\x01'`, []string{`transactions_status_pkey`}},
	{`SELECT "hash" FROM "log_transactions" WHERE "hash" IN ('\x01') UNION ALL SELECT "hash" FROM "stop_network_txs" WHERE "hash" IN ('\x01')`,
		[]string{`log_transactions_p\d+_hash_idx`, `stop_network_txs_pkey`}},
	{`SELECT "count" FROM "log_transaction_counts" WHERE "ecosystem_id" = 1`, []string{`log_transaction_counts_pkey`}},
	{`SELECT si.output_tx_hash FROM spent_info si WHERE si.output_key_id IN (1, 2) AND si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index`, []string{`spent_info_unspent`}},
	{`SELECT si.output_tx_hash FROM spent_info si WHERE si.ecosystem = 1 AND si.output_key_id IN (1, 2) AND si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index`, []string{`spent_info_unspent`}},
	{`SELECT coalesce(sum(output_value), '0') FROM spent_info WHERE input_tx_hash is NULL AND output_key_id = 1 AND ecosystem = 1`,
		[]string{`spent_info_unspent`}},
	{`SELECT * FROM "block_chain" ORDER BY "block_chain"."id" DESC LIMIT 1`, []string{`block_chain_p\d+_pkey`}},
	{`SELECT * FROM "block_chain" WHERE id = 1`, []string{`block_chain_genesis_pkey`}},
	{`SELECT * FROM "block_chain" WHERE node_position = 0 AND time BETWEEN 1 AND 2`,
		[]string{`block_chain_p\d+_node_position_time_idx`}},
	{`SELECT * FROM "confirmations" WHERE block_id = 1`, []string{`confirmations_p0_pkey`}},
	{`SELECT * FROM "rollback_tx" WHERE block_id = 1 AND tx_hash = '\x01' ORDER BY id DESC`, []string{`rollback_tx_p0_`}},
	{`SELECT * FROM "rollback_tx" WHERE table_name = '1_keys' AND table_id = '1' ORDER BY block_id DESC`,
		[]string{`rollback_tx_p\d+_table_name_table_id_block_id_idx`}},
	{`SELECT * FROM "1_history" WHERE txhash = '\x01'`, []string{`1_history_p\d+_txhash_idx`}},
	{`select ap.value from "1_app_params" ap, "1_applications" a where ap.app_id = a.id and a."name" = 'CandidateNode'
		and ap."name" = 'limit_candidate_pack' and a.deleted = 0 and a.ecosystem = 1`,
		[]string{`1_app_params_ecosystem_app_id_name_idx`}},
	{`SELECT k.id, count(n.id) FROM "1_keys" k LEFT JOIN "1_notifications" n ON n.ecosystem = k.ecosystem AND n.closed = 0
		AND n.notification->>'type' = '1' and n.recipient->>'account' = k.account
		WHERE k.ecosystem = 1 AND k.account = '0000-0000-0000-0000-0000' GROUP BY k.id`,
		[]string{`1_keys_ecosystem_account`, `1_notifications_account`}},
	{`SELECT k.id, rp.role->>'id', count(n.id) FROM "1_keys" k
		INNER JOIN "1_roles_participants" rp ON rp.member->>'account' = k.account
		LEFT JOIN "1_notifications" n ON n.ecosystem = k.ecosystem AND n.closed = 0 AND n.notification->>'type' = '2'
			AND n.recipient->>'role_id' = rp.role->>'id'
		WHERE k.ecosystem = 1 AND k.account = '0000-0000-0000-0000-0000' GROUP BY k.id, rp.role->>'id'`,
		[]string{`1_roles_participants_account`, `1_notifications_role`}},
}

var planIndex = regexp.MustCompile(`Index(?: Only)? Scan(?: Backward)? using "?([^" ]+)"?|Bitmap Index Scan on "?([^" ]+)"?`)

// TestHotQueriesUseIndexes: with sequential scans disabled the planner takes the cheapest index, so
// that each query reads by the index it is meant to read by, whatever the size of the tables, and
// by none whose condition leaves the rest to a filter
func TestHotQueriesUseIndexes(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Exec(`SET LOCAL enable_seqscan = off`).Error)
	for _, q := range hotQueries {
		var plan []string
		require.NoError(t, db.Raw(`EXPLAIN `+q.sql).Scan(&plan).Error, q.sql)
		text := strings.Join(plan, "\n")
		require.NotContains(t, text, "Seq Scan", q.sql)
		var used []string
		for _, m := range planIndex.FindAllStringSubmatch(text, -1) {
			used = append(used, m[1]+m[2])
		}
		for _, want := range q.indexes {
			re := regexp.MustCompile(`^` + want)
			require.Truef(t, func() bool {
				for _, u := range used {
					if re.MatchString(u) {
						return true
					}
				}
				return false
			}(), "%s\nreads by %v, not %s", q.sql, used, want)
		}
	}
}
