# Data queries

The node's REST API and JSON-RPC read ecosystem tables through one implementation,
`packages/dataquery`. A read sees only what the table's read conditions allow the reader. Every
value in a query is bound, never written into the SQL text. Every name in a query must be one of
the table's columns. Every cell in a response is typed.

## Tables and readers

A table is named `name` or `@<ecosystem>name`.

- If the name has no prefix, the query's `ecosystem` field picks the ecosystem.
- If that field is also missing, the ecosystem of the session is used.
- Tables of ecosystem 1 that hold the rows of every ecosystem (`converter.FirstEcosystemTables`)
  are read only within the chosen ecosystem.

The reader is the key of the session. Read conditions come from the table's row in `1_tables`:

- `permissions.read` covers the whole table. A table without it can be read by everyone.
- `columns.<column>` holds either an update condition, or `{"update": ..., "read": ...}`. That
  value may be a JSON object or its text. A column without a `read` can be read by everyone who can
  read the table.

A condition is evaluated in the table's ecosystem, with the reader's key. If it fails to evaluate,
it counts as not holding.

A table that exists in the database but has no row in `1_tables` belongs to the node itself. It is
never read through this API (`E_ACCESS_DENIED`).

Every table in the genesis data of a new chain declares `permissions.read`.

## Endpoints

| REST                                  | JSON-RPC                       | Reads                                             |
| ------------------------------------- | ------------------------------ | ------------------------------------------------- |
| `POST listWhere/{table}` (JSON body)  | `getList({name, ...query})`    | a page of rows                                    |
| `GET list/{table}`                    |                                | the same query: `where`, `order` as JSON in the URL |
| `GET row/{table}/{id}`                | `getRow(name, value, columns, column, ecosystem)` | one row                       |
| `GET row/{table}/{column}/{value}`    |                                | the row whose column holds the value              |
| `POST sumWhere/{table}` (JSON body)   |                                | `{"sum": "<decimal>"}` of a numeric column        |
| `GET table/{table}`                   | `getTable(name, ecosystem)`    | the table's permissions and columns (public)      |
| `GET history/{table}/{id}`            |                                | past values of a row, readable columns only       |

REST GETs take `ecosystem` and `columns` (comma separated) as URL parameters.

A query body looks like this:

```json
{
  "ecosystem": 1,
  "columns": ["id", "name", "data->title"],
  "where": { "amount": { "$gte": "10.5" }, "$or": [{ "name": { "$begin": "a" } }, { "id": 7 }] },
  "order": [{ "name": "asc" }],
  "limit": 25,
  "offset": 0
}
```

- `columns`: when empty, every column the reader may read is returned. A named column the reader
  may not read is refused. `column->key->key` reads a path within a `json` column; keys match
  `[a-z0-9_]`.
- `limit`: 1 to 1000, 25 by default. `offset`: 0 to 1000000.
- `order`: a list of `{column: "asc" | "desc"}`. It always ends with `id` ascending, so pages never
  overlap.

A page looks like this:

```json
{
  "count": 120,
  "columns": [{ "name": "id", "type": "number" }, { "name": "name", "type": "text" }],
  "list": [{ "id": 1, "name": "a" }]
}
```

Each row holds exactly the columns announced in `columns`. `count` is the number of rows the
filter matches.

## Where

| Form                                        | Meaning                                             |
| ------------------------------------------- | --------------------------------------------------- |
| `{"col": value}`                            | `$eq`                                               |
| `{"col": {"$eq" / "$neq": null}}`           | is (not) null                                       |
| `$eq $neq $gt $gte $lt $lte`                | comparison with a value of the column's type        |
| `$in $nin`                                  | a list of values. `$in []` matches nothing, `$nin []` everything |
| `$like $begin $end`, `$ilike $ibegin $iend` | contains / starts with / ends with; `i` is case-insensitive. `%` and `_` are literal |
| `$and`, `$or`                               | a list of filters. `$and []` matches everything, `$or []` nothing |

A filter's keys are joined with AND.

- A path `col->key` is compared as text.
- A `json` column takes only `$eq` and `$neq`.
- A filter is limited to 16 levels of nesting and 256 conditions.

## Types

| Type        | Columns                       | JSON value                                                       |
| ----------- | ----------------------------- | ---------------------------------------------------------------- |
| `number`    | bigint, integer, smallint     | a number, or a decimal string beyond 2^53-1                      |
| `money`     | numeric                       | a decimal string                                                 |
| `double`    | double precision, real        | a number                                                         |
| `json`      | jsonb, json, boolean          | the JSON value                                                   |
| `timestamp` | timestamp, date               | ISO 8601 in UTC, `2026-10-09T08:00:00.000000Z`                   |
| `bytes`     | bytea                         | lowercase hex                                                    |
| `text`      | any other                     | a string                                                         |

A value in a filter must be of its column's type:

- `number`: an integer or an integer string.
- `money`: a decimal number or string.
- `timestamp`: RFC 3339, `YYYY-MM-DDTHH:MM:SS` (UTC) or `YYYY-MM-DD`.
- `bytes`: hex.

## Errors

REST answers `{"error": code, "msg": detail}` with the HTTP status below. JSON-RPC answers an error
whose `data.error` is the code.

| Code              | Status | When                                                      |
| ----------------- | ------ | --------------------------------------------------------- |
| `E_ACCESS_DENIED` | 403    | a read condition of the table, or of a named column, does not hold |
| `E_COLUMN`        | 400    | a column the table does not have, or one named twice      |
| `E_VALUE`         | 400    | a value that is not of its column's type                  |
| `E_WHERE`         | 400    | a filter, order or body outside this language             |
| `E_LIMIT`         | 400    | a limit or offset out of range                            |
| `E_TABLENOTFOUND` | 404    | no such table                                             |
| `E_NOTFOUND`      | 404    | no such row                                               |
| `E_QUERY`         | 500    | a database failure, logged by the node                    |
