# UTXO movements

The node lists what each transaction did to an account's UTXO in one ecosystem: the transfers it
sent and received, its moves between its account and its UTXO, the fees it paid in this ecosystem
for a transfer in another, the packaging and taxes it was paid, and the genesis output. Wallets read
the history from the chain this way, without an indexer.

## Endpoints

| REST                  | JSON-RPC             |
| --------------------- | -------------------- |
| `GET utxo/{account}`  | `ibax.utxoMovements` |

`account` is an address or a key ID. Like the balance, the movements are public: no session is
needed.

| REST        | JSON-RPC       | Meaning                                                            |
| ----------- | -------------- | ------------------------------------------------------------------ |
| `ecosystem` | `ecosystem_id` | the ecosystem; 1 when missing                                       |
| `before`    | `before`       | only blocks before this one; 0 or missing for the latest            |
| `limit`     | `limit`        | how many movements at least, 1 to 100; 25 when missing              |

A page holds whole blocks, the latest first: once `limit` movements are found, the rest of the last
block is added. The next page is asked for with `before` = the `block` of the last movement, so
paging neither repeats nor skips one.

```json
{
  "movements": [
    {
      "hash": "5d1e…",
      "block": "1043",
      "time": "2026-10-09T08:00:00Z",
      "kind": "transfer",
      "sender": "-6097185355090423139",
      "recipient": "3201477082340138571",
      "amount": "1000000000000",
      "fee": "1200000",
      "comment": "rent",
      "net": "1000000000000"
    }
  ],
  "more": true
}
```

| Field       | Meaning                                                                          |
| ----------- | -------------------------------------------------------------------------------- |
| `hash`      | the transaction, hex                                                              |
| `block`     | its block, decimal                                                                |
| `time`      | the block's time, RFC 3339                                                        |
| `kind`      | see below                                                                         |
| `to`        | for `move` only: `utxo` or `account`, where the tokens went                       |
| `sender`    | the key that sent the transaction; `0` for `genesis`                              |
| `recipient` | the transfer's recipient; the account itself otherwise                            |
| `amount`    | what was sent or moved; for `reward` and `genesis` what the account got; for `fee` 0 |
| `fee`       | the packaging, taxes and combustion the transaction paid in this ecosystem        |
| `comment`   | the transfer's comment, as signed                                                 |
| `net`       | what the account's UTXO gained, negative when it lost                             |
| `more`      | whether earlier blocks have movements                                             |

Amounts are in base units. Key IDs, block numbers and amounts are decimal strings.

## Kinds

| Kind       | The transaction                                                                    |
| ---------- | ---------------------------------------------------------------------------------- |
| `genesis`  | is the first block, which made the output                                          |
| `transfer` | is a UTXO transfer in this ecosystem that the account sent or received              |
| `move`     | moved the account's tokens between its account and its UTXO; no fee                 |
| `fee`      | is the account's UTXO transfer in another ecosystem, whose fee it paid here         |
| `reward`   | is another's UTXO transfer that paid the account packaging, as the block's producer, or taxes |

A recipient that also made the block has one `transfer` whose `net` is both.

## How

The node keeps every UTXO output in `spent_info`, with the transaction that made it and the one
that spent it. The movements are the transactions that made an output to the account or spent one
of its, in the ecosystem; each is classified by what its block holds of it. The index
`(ecosystem, output_key_id, block_id)` serves the query; existing nodes create it at start. A
rolled back block's outputs are deleted with it, so its movements are gone too.

## Errors

| Code           | HTTP | When                                   |
| -------------- | ---- | -------------------------------------- |
| `E_UTXOPAGING` | 400  | `before` is negative, or `limit` is out of range |
| `E_INVALIDWALLET` | 400 | `account` is no address or key ID   |
| `E_SERVER`     | 400  | the outputs name a transaction no block holds |

JSON-RPC answers invalid parameters with -32010.
