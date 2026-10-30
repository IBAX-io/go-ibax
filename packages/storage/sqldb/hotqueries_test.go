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
// an index of a partition is matched by its name with the range as p\d+. An account is in a few
// ecosystems, so reading it by its account alone is as good as by both
var hotQueries = []struct {
	sql     string
	indexes []string
}{
	{`SELECT * FROM "1_keys" WHERE id = 1 and ecosystem = 1 ORDER BY "1_keys"."id" LIMIT 1`, []string{`1_keys_pkey`}},
	{`SELECT id, amount, "ecosystem" FROM "1_keys" WHERE "ecosystem" = 1 and "account" = '0000-0000-0000-0000-0000' and "deleted" = 0`,
		[]string{`1_keys_(ecosystem_account|account_idx)$`}},
	{`UPDATE "1_keys" SET amount = amount + 1 WHERE "ecosystem" = 1 and "id" = 1`, []string{`1_keys_pkey`}},
	{`SELECT * FROM "1_ecosystems" WHERE id = 1`, []string{`1_ecosystems_pkey`}},
	// 1_parameters and 1_app_params have a unique index and a plain one on the same columns: either serves
	{`SELECT * FROM "1_parameters" WHERE ecosystem = 1 and name = 'utxo_fee'`, []string{`1_parameters_ecosystem_name`}},
	{`SELECT * FROM "1_members" WHERE "ecosystem" = 1 and "account" = '0000-0000-0000-0000-0000'`,
		[]string{`1_members_account_ecosystem_idx`}},
	// an ecosystem has tens of applications
	{`SELECT * FROM "1_applications" WHERE "ecosystem" = 1 and "name" = 'Basic' and "deleted" = 0`,
		[]string{`1_applications_ecosystem_idx`}},
	{`SELECT * FROM "transactions_status" WHERE hash = '\x01'`, []string{`transactions_status_pkey`}},
	{`SELECT "hash" FROM "log_transactions" WHERE "hash" IN ('\x01') UNION ALL SELECT "hash" FROM "stop_network_txs" WHERE "hash" IN ('\x01')`,
		[]string{`log_transactions_p\d+_hash_idx`, `stop_network_txs_pkey`}},
	{`SELECT "count" FROM "log_transaction_counts" WHERE "ecosystem_id" = 1`, []string{`log_transaction_counts_pkey`}},
	{`SELECT si.output_tx_hash FROM spent_info si WHERE si.output_key_id IN (1, 2) AND si.spent_block = 0
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index`, []string{`spent_info_unspent_output_key_id_ecosystem_idx`}},
	{`SELECT si.output_tx_hash FROM spent_info si WHERE si.ecosystem = 1 AND si.output_key_id IN (1, 2) AND si.spent_block = 0
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index`, []string{`spent_info_unspent_output_key_id_ecosystem_idx`}},
	{`SELECT coalesce(sum(output_value), '0') FROM spent_info WHERE spent_block = 0 AND output_key_id = 1 AND ecosystem = 1`,
		[]string{`spent_info_unspent_output_key_id_ecosystem_idx`}},
	{`UPDATE "spent_info" SET "input_tx_hash" = u.i, "input_index" = u.ii, "spent_block" = u.s
		FROM unnest('{1}'::bigint[], '{"\\x01"}'::bytea[], '{1}'::bigint[], '{0}'::integer[], '{"\\x02"}'::bytea[],
			'{0}'::integer[], '{2}'::bigint[]) AS u(b, o, k, oi, i, ii, s)
		WHERE "block_id" = u.b AND "output_tx_hash" = u.o AND "output_key_id" = u.k AND "output_index" = u.oi
			AND "spent_block" = 0`, []string{`spent_info_unspent_pkey`}},
	{`UPDATE spent_info SET input_tx_hash = null, input_index = 0, spent_block = 0 WHERE spent_block = 1 AND spent_block > 0`,
		[]string{`spent_info_p0_spent_block_input_tx_hash_idx`}},
	{`DELETE FROM spent_info WHERE block_id = 1`, []string{`spent_info_unspent_pkey`, `spent_info_p\d+_pkey`}},
	{`SELECT * FROM spent_info WHERE (block_id, output_tx_hash) IN ((1, '\x01'), (2, '\x02')) AND ecosystem = 1`,
		[]string{`spent_info_unspent_pkey`, `spent_info_p\d+_pkey`}},
	{`SELECT * FROM spent_info WHERE (spent_block, input_tx_hash) IN ((1, '\x01'), (2, '\x02')) AND spent_block > 0
		AND ecosystem = 1 AND output_key_id = 1`, []string{`spent_info_p0_spent_block_input_tx_hash_idx`}},
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
		[]string{`1_app_params_ecosystem_app_id_name`}},
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

// hotRows are the rows of the tables of the hot queries at the shape of a chain under load: most keys
// and their transactions in ecosystem 1, a thousand ecosystems with their parameters and
// applications, 5000 blocks of 10 transactions, each making 4 outputs, the first two the packaging and
// taxes to keys 1 and 2, and 95% of the outputs spent
var hotRows = []string{
	`INSERT INTO "1_ecosystems" (id, name) SELECT g, 'e' || g FROM generate_series(1, 1000) g`,
	`INSERT INTO "1_keys" (id, ecosystem, account) SELECT g, CASE WHEN g % 10 = 0 THEN 2 + g % 999 ELSE 1 END,
		lpad(g::text, 24, '0') FROM generate_series(1, 100000) g`,
	`INSERT INTO "1_members" (id, ecosystem, account) SELECT g, CASE WHEN g % 10 = 0 THEN 2 + g % 999 ELSE 1 END,
		lpad(g::text, 24, '0') FROM generate_series(1, 100000) g`,
	`INSERT INTO "1_parameters" (id, ecosystem, name) SELECT g, 1 + g / 30, 'p' || g % 30 FROM generate_series(0, 29999) g`,
	`INSERT INTO "1_applications" (id, ecosystem, name) SELECT g, 1 + g / 20, 'a' || g % 20 FROM generate_series(0, 19999) g`,
	`INSERT INTO "1_app_params" (id, ecosystem, app_id, name) SELECT g, 1 + g / 100, g / 5, 'n' || g % 5
		FROM generate_series(0, 99999) g`,
	`INSERT INTO "1_notifications" (id, ecosystem, closed, recipient, notification)
		SELECT g, CASE WHEN g % 10 = 0 THEN 2 + g % 999 ELSE 1 END, (g % 3 = 0)::int,
			jsonb_build_object('account', lpad((g % 100000)::text, 24, '0'), 'role_id', (g % 50)::text),
			jsonb_build_object('type', 1 + g % 2) FROM generate_series(1, 50000) g`,
	`INSERT INTO "1_roles_participants" (id, ecosystem, role, member) SELECT g, 1, jsonb_build_object('id', (g % 50)::text),
		jsonb_build_object('account', lpad(g::text, 24, '0')) FROM generate_series(1, 50000) g`,
	`INSERT INTO "log_transaction_counts" (ecosystem_id, count) SELECT g, g FROM generate_series(1, 1000) g`,
	`INSERT INTO "block_chain" (id, hash, node_position, time) SELECT g, sha256(int8send(g)), g % 5, g
		FROM generate_series(1, 5001) g`,
	`INSERT INTO "confirmations" (block_id) SELECT g FROM generate_series(1, 5001) g`,
	`INSERT INTO "transactions_status" (hash, block_id) SELECT sha256(int8send(g)), 2 + g / 20
		FROM generate_series(0, 99999) g`,
	`INSERT INTO "log_transactions" (hash, block, ecosystem_id) SELECT sha256(int8send(g)), 2 + g / 20, 1
		FROM generate_series(0, 99999) g`,
	`INSERT INTO "stop_network_txs" (hash, block) SELECT sha256(int8send(-g)), g FROM generate_series(1, 10) g`,
	`INSERT INTO "1_history" (id, block_id, txhash) SELECT g, 2 + g / 20, sha256(int8send(g))
		FROM generate_series(0, 99999) g`,
	`INSERT INTO "rollback_tx" (id, block_id, tx_hash, table_name, table_id)
		SELECT g, 2 + g / 40, sha256(int8send(g / 2)), '1_keys', (g % 100000)::text FROM generate_series(0, 199999) g`,
	`INSERT INTO "spent_info" (output_tx_hash, output_index, output_key_id, output_value, ecosystem, block_id, type,
			input_tx_hash, input_index, spent_block)
		SELECT sha256(int8send(g / 4)), g % 4, CASE WHEN g % 4 < 2 THEN 1 + g % 4 ELSE 3 + g * 7919 % 100000 END,
			1, 1, 2 + g / 40, 0,
			CASE WHEN g % 20 = 0 THEN NULL ELSE sha256(int8send(200000 + g)) END, 0,
			CASE WHEN g % 20 = 0 THEN 0 ELSE 3 + g / 40 + g % 7 END
		FROM generate_series(0, 199999) g`,
}

// hotTables are the tables hotRows fills
var hotTables = []string{"1_ecosystems", "1_keys", "1_members", "1_parameters", "1_applications", "1_app_params",
	"1_notifications", "1_roles_participants", "log_transaction_counts", "block_chain", "confirmations",
	"transactions_status", "log_transactions", "stop_network_txs", "1_history", "rollback_tx", "spent_info"}

var planIndex = regexp.MustCompile(`Index(?: Only)? Scan(?: Backward)? using "?([^" ]+)"?|Bitmap Index Scan on "?([^" ]+)"?`)

// TestHotQueriesUseIndexes: on tables of the shape of a chain under load, whatever rows and
// statistics the database had before, and with sequential scans disabled, the planner takes the
// cheapest index, so that each query reads by the index it is meant to read by, and by none whose
// condition leaves the rest to a filter
func TestHotQueriesUseIndexes(t *testing.T) {
	db := testDB(t)
	for _, table := range hotTables {
		require.NoError(t, db.Exec(`DELETE FROM "`+table+`"`).Error)
	}
	for _, rows := range hotRows {
		require.NoError(t, db.Exec(rows).Error, rows)
	}
	for _, table := range hotTables {
		require.NoError(t, db.Exec(`ANALYZE "`+table+`"`).Error)
	}
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
