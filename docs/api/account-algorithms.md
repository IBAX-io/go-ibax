# Account algorithms

Each account signs with the algorithm of its own key, so accounts of different algorithms share
one chain. The node keys (`honor_nodes`, block signatures, the `NodePublicKey` file) keep the
algorithm of the network, `cryptoer` in `getuid`.

## Account keys

An account key, in `1_keys.pub`, the transaction header, the login form and `@1NewUser`, is

```
0xAC | algorithm (one byte) | public key
```

| Algorithm       | Byte | Public key                     |
| --------------- | ---- | ------------------------------ |
| `ECC_P256`      | 0    | 64 bytes, X and Y              |
| `ECC_Secp256k1` | 1    | 64 bytes, X and Y              |
| `SM2`           | 2    | 64 bytes, X and Y              |
| `MLDSA65`       | 4    | 1952 bytes                     |
| `MLDSA87`       | 5    | 2592 bytes                     |

The address is that of the public key without the first two bytes. A key that is not the marker,
a known algorithm and a valid public key of it is refused; a bare curve key (`04` and X, Y) is no
account key. The node account signs with the node key: its account key is the node algorithm and
the node public key.

## account_algorithms

The platform parameter lists the algorithms account keys may have:

```json
[{ "algo": "ECC_P256", "register_until": "2030-12-31", "sign_until": "2031-12-31" }, { "algo": "MLDSA65" }]
```

- A key signs (transactions, logins, the contract function `CheckSign`) while its algorithm is in
  the list and the block time is on or before `sign_until`.
- A key registers (a new key signing `@1NewUser`, the contract function `HexToPub`, a login with a
  key not registered) on or before `register_until` as well.
- Days are whole UTC days. A missing day is no limit.
- A node refuses a transaction it is sent (`sendTx`) whose header key may not sign by its clock;
  once in a block, the block time decides.
- A new value may move a day earlier or add one, never later or away; an algorithm is removed
  only when no key, not deleted, has it.

## Contract and template functions

| Function                | Takes                                         | Gives                                       |
| ----------------------- | --------------------------------------------- | ------------------------------------------- |
| `PubToID(hex)`          | an account key                                | its address, 0 for what is no account key   |
| `HexToPub(hex)`         | an account key that may still register        | the bytes for `1_keys.pub`; an error otherwise |
| `PubToHex(bytes)`       | bytes                                          | their hex                                   |
| `CheckSign(pub, data, sign)` | an account key that may still sign, hex  | whether the signature verifies              |
| `NodePubToID(hex)`      | a node key as `honor_nodes` holds it          | the address of the node, 0 for what is no node key |
| `NodePubKey(bytes)`     | the `1_keys.pub` of a node account            | its node key as `honor_nodes` holds it; an error for a key of another algorithm |

The templates have `PubToID` and `NodePubToID` alike.

## Endpoints

| REST                                          | JSON-RPC                                     |
| --------------------------------------------- | -------------------------------------------- |
| `GET accountalgorithms`                       | `ibax.accountAlgorithms`                     |
| `GET accountalgorithms/{algo}/keys?limit=&offset=` | `ibax.accountAlgorithmKeys(algo, limit, offset)` |

Neither needs a session. `accountalgorithms` is `account_algorithms` with the number of keys, not
deleted, of each algorithm in every ecosystem:

```json
[
  { "algo": "ECC_P256", "register_until": "2030-12-31", "sign_until": "2031-12-31", "keys": 1520 },
  { "algo": "MLDSA65", "keys": 12 }
]
```

`accountalgorithms/{algo}/keys` lists the accounts whose keys still have the algorithm, by
ecosystem and account: the keys to rebind before it stops signing. `limit` is 25 by default and
at most 100.

```json
{ "count": 1520, "list": [{ "account": "1234-5678-9012-3456-7890", "ecosystem": 1 }] }
```

An algorithm that is no account algorithm is refused with 400 (JSON-RPC -32010).
