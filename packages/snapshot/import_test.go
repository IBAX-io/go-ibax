/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapshot

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// the state of the tests of the import: every kind of object of the schema it creates
const fullSchema = schema + `;
CREATE FUNCTION next_id(t text) RETURNS bigint LANGUAGE plpgsql AS $$ BEGIN RETURN 1; END $$;
CREATE TABLE "1_owner" (id bigint PRIMARY KEY DEFAULT next_id('1_owner'), name text COLLATE "C" NOT NULL UNIQUE,
	CONSTRAINT long CHECK (length(name) > 0));
CREATE TABLE "1_pet" (id bigint PRIMARY KEY, owner bigint REFERENCES "1_owner" (id), at bigint NOT NULL DEFAULT 0);
CREATE INDEX ON "1_pet" (owner, at) WHERE at > 0;
CREATE INDEX ON "1_history" (note);
CREATE INDEX ON "1_history_0" (id);
CREATE INDEX ON info_block (sent);
CREATE FUNCTION stamp() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.at = NEW.at + 1; RETURN NEW; END $$;
CREATE TRIGGER stamp BEFORE INSERT ON "1_pet" FOR EACH ROW EXECUTE FUNCTION stamp();
CREATE VIEW pets AS SELECT p.id, o.name FROM "1_pet" p JOIN "1_owner" o ON o.id = p.owner`

const fullRows = `INSERT INTO "1_keys" VALUES (1, 1, 100, '\x01', '{"b":1}'), (2, 1, 5, NULL, NULL);
INSERT INTO "1_names" VALUES ('b', '2030-01-01 08:00:00+08', 0.1), ('a', NULL, -0.0);
INSERT INTO "1_history" VALUES (1, 5, 'x'), (2, 150, E'line\nnext\ttab');
INSERT INTO "1_bag" VALUES ('z', 1), (NULL, 3);
INSERT INTO info_block VALUES ('\xaa', 7, 1);
INSERT INTO block_chain VALUES (1, '\x00');
INSERT INTO "1_owner" (name) VALUES ('o');
INSERT INTO "1_pet" VALUES (1, 1, 0), (2, NULL, 5)`

// snapshotTo writes the snapshot of a height of the database in a store
func snapshotTo(t *testing.T, db *sql.DB, store Store, height int64) *State {
	t.Helper()
	s, err := Begin(context.Background(), db)
	require.NoError(t, err)
	defer s.Close()
	files, err := store.Create(height)
	require.NoError(t, err)
	st, err := s.Hash(context.Background(), 2, files.Sink())
	require.NoError(t, err)
	require.NoError(t, files.Commit(st))
	return st
}

func count(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(q).Scan(&n))
	return n
}

// A database of other state takes that of a snapshot: the same hash, rows and objects that work
// as those of the source, its node tables as they were and its node columns as it made them, of
// their default
func TestImport(t *testing.T) {
	src, dst := testDB(t), testDB(t)
	exec(t, src, fullSchema, fullRows)
	store := Store{Dir: t.TempDir()}
	st := snapshotTo(t, src, store, 10000)

	exec(t, dst, `CREATE TABLE block_chain (id bigint PRIMARY KEY, data bytea)`, `INSERT INTO block_chain VALUES (1, '\x01'), (2, '\x02')`,
		`CREATE TABLE info_block (hash bytea, block_id integer, sent smallint NOT NULL DEFAULT 2)`, `INSERT INTO info_block VALUES ('\x00', 1, 0)`,
		`CREATE TABLE "1_keys" (id bigint PRIMARY KEY)`, `CREATE TABLE "1_old" (v text)`,
		`CREATE FUNCTION next_id(t text) RETURNS bigint LANGUAGE sql AS 'SELECT 7'`, `CREATE FUNCTION gone() RETURNS int LANGUAGE sql AS 'SELECT 1'`,
		`CREATE VIEW old AS SELECT * FROM "1_old"`)
	require.NoError(t, Import(context.Background(), dst, store, 10000, st.Hash, 2))
	require.Equal(t, st.Hash, hashOf(t, dst, 1).Hash)
	require.Equal(t, 2, count(t, dst, `SELECT count(*) FROM block_chain`))
	require.Equal(t, 0, count(t, dst, `SELECT count(*) FROM pg_class WHERE relname IN ('1_old', 'old')`))
	require.Equal(t, 1, count(t, dst, `SELECT count(*) FROM pets`))
	require.Equal(t, 2, count(t, dst, `SELECT sent FROM info_block`))
	require.Equal(t, 1, count(t, dst, `SELECT count(*) FROM pg_indexes WHERE tablename = 'info_block' AND indexdef LIKE '%(sent)'`))
	_, err := dst.Exec(`UPDATE info_block SET sent = NULL`)
	require.ErrorContains(t, err, "not-null")
	// the trigger fires on what comes after the copy alone, and the constraints hold
	exec(t, dst, `INSERT INTO "1_pet" VALUES (3, 1, 0)`)
	require.Equal(t, 1, count(t, dst, `SELECT at FROM "1_pet" WHERE id = 3`))
	require.Equal(t, 6, count(t, dst, `SELECT at FROM "1_pet" WHERE id = 2`))
	_, err = dst.Exec(`INSERT INTO "1_pet" VALUES (4, 9, 0)`)
	require.ErrorContains(t, err, "foreign key")
	_, err = dst.Exec(`INSERT INTO "1_owner" (id, name) VALUES (2, 'o')`)
	require.ErrorContains(t, err, "unique")
	exec(t, dst, `INSERT INTO "1_history" VALUES (3, 199, 'y')`)
	require.Equal(t, 1, count(t, dst, `SELECT count(*) FROM "1_history_1" WHERE id = 3`))

	// an import again, over the state it made
	require.NoError(t, Import(context.Background(), src, store, 10000, st.Hash, 2))
	require.Equal(t, st.Hash, hashOf(t, src, 1).Hash)
}

// The import fails before it changes anything of a database without the node columns
func TestImportNoNodeColumn(t *testing.T) {
	src, dst := testDB(t), testDB(t)
	exec(t, src, fullSchema, fullRows)
	store := Store{Dir: t.TempDir()}
	st := snapshotTo(t, src, store, 10000)
	exec(t, dst, `CREATE TABLE info_block (hash bytea, block_id integer)`, `CREATE TABLE "1_old" (v text)`)
	require.ErrorContains(t, Import(context.Background(), dst, store, 10000, st.Hash, 2), "no column sent of info_block")
	require.Equal(t, 1, count(t, dst, `SELECT count(*) FROM pg_class WHERE relname = '1_old'`))
}

// The import fails of the files of other rows than the manifest, and of a manifest of another
// state hash
func TestImportTampered(t *testing.T) {
	src, dst := testDB(t), testDB(t)
	exec(t, src, fullSchema, fullRows)
	store := Store{Dir: t.TempDir()}
	st := snapshotTo(t, src, store, 10000)
	exec(t, dst, `CREATE TABLE info_block (hash bytea, block_id integer, sent smallint)`)
	other := append([]byte{}, st.Hash...)
	other[0] ^= 1
	require.ErrorContains(t, Import(context.Background(), dst, store, 10000, other, 2), "not of the state hash")
	require.ErrorContains(t, Import(context.Background(), dst, store, 20000, st.Hash, 2), "no snapshot")

	path := store.path(10000, "1_bag"+tableSuffix)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data[0] = 'y'
	require.NoError(t, os.WriteFile(path, data, 0o644))
	require.ErrorContains(t, Import(context.Background(), dst, store, 10000, st.Hash, 2), "1_bag of other rows")
}
