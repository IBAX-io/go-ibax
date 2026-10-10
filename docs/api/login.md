# Login

A client opens a session of the node's REST API and JSON-RPC through one implementation,
`packages/login`. The node issues a one-time challenge, the client signs it with its key, and the
node checks the signature before it reads anything else. A login never writes to the chain, and the
node never signs or pays for a client.

## Endpoints

| REST                     | JSON-RPC       | Does                                                      |
| ------------------------ | -------------- | --------------------------------------------------------- |
| `GET getuid`             | `ibax.getUid`  | issues a challenge, or says what the session token stands for |
| `POST login` (form)      | `ibax.login`   | checks the signed challenge and opens a session           |

Both take the token of the previous step as `Authorization: Bearer <token>`.

## Challenge

Without a session token, `getuid` answers:

```json
{
  "uid": "084172590385372716209548235012764501337",
  "token": "<challenge token>",
  "network_id": "1",
  "cryptoer": "ECC_P256",
  "hasher": "SHA256",
  "fips": false
}
```

- `uid` is 128 bits from `crypto/rand`, written as 39 decimal digits with leading zeros. The client
  signs only `"LOGIN" + network_id + uid`, and checks that `uid` is digits only, so a node cannot
  make it sign anything else.
- `token` carries the `uid` and a random `jti`. It expires with the challenge.
- A challenge lives 30 seconds (`login.ChallengeLifetime`) in the memory of the node that issued
  it. It is used on that node only.
- At most 100,000 challenges (`login.MaxChallenges`) are open at once. Past that, `getuid` answers
  `E_BUSY` until expired challenges are swept.

With a session token, `getuid` answers its `ecosystem_id`, `key_id`, `address` and the time left
(`expire`), and issues no challenge.

## Login

| REST field  | JSON-RPC field  | Meaning                                                        |
| ----------- | --------------- | -------------------------------------------------------------- |
| `ecosystem` | `ecosystem_id`  | the ecosystem of the session; 1 when missing                    |
| `expire`    | `expire`        | session lifetime in seconds, 0 to 28800; 0 or missing means 28800 |
| `pubkey`    | `public_key`    | the public key, hex                                            |
| `key_id`    | `key_id`        | the key's address, when `pubkey` is missing                    |
| `signature` | `signature`     | signature of `"LOGIN" + network_id + uid`, hex                  |
| `role_id`   | `role_id`       | a role of the account to act in; 0 for none                     |
| `guest`     | `guest`         | sign in as the guest account, without a signature               |

The node checks, in this order:

1. The challenge in the token is open and unexpired. It is closed now, whatever the outcome: a
   second login with it gets `E_UNKNOWNUID`, even after a failed one.
2. `expire` is within range.
3. The key: from `pubkey`, or from the chain's keys table by `key_id`. A `key_id` that is not the
   address of `pubkey` is refused.
4. The signature, with the chain's public key of the account when it has one, otherwise with
   `pubkey`. The session is of the key that verified the signature.
5. The account: a deleted key is refused; a key the ecosystem does not know gets `E_NEWUSER`, or
   `E_ECONOTOPEN` when the ecosystem takes no new members (`free_membership`).
6. `role_id`, when given: the account must be a member of the role.

The answer:

```json
{
  "token": "<session token>",
  "ecosystem_id": "1",
  "key_id": "-6097185355090423139",
  "account": "1234-5678-9012-3456-7890",
  "notify_key": "<centrifugo connection token>",
  "isnode": false,
  "isowner": false,
  "clb": false,
  "timestamp": "1760000000",
  "roles": [{ "role_id": 3, "role_name": "Developer" }]
}
```

`notify_key` connects the session to Centrifugo for the account's notifications until the session
expires; see [notifications](notifications.md).

### Guest

`guest=true` opens a session of the guest account (`consts.GuestKey`) in the ecosystem asked for.
`pubkey`, `key_id`, `signature` and `role_id` are ignored: the guest key is public, so a signature
with it proves nothing. Clients use it to read a network without signing in software. Every node
accepts it, in every mode.

### New keys

The node does not register keys. A key the ecosystem does not know registers itself:

1. Login answers `E_NEWUSER`.
2. The client sends `@1NewUser` signed with that key: header ecosystem 1, the public key in the
   header, `NewPubkey` = the same key in hex, and `Ecosystem` = the ecosystem to join.
3. Once the transaction is in a block, the client asks for a new challenge and logs in again.

The chain accepts a transaction from a key without a keys row for `@1NewUser` only, and only when
the key signs it itself (`SignedBy` 0, the public key in the header). The contract refuses a
`NewPubkey` other than the signer's. `@1NewUser` is in the platform parameter `pay_free_contract`,
so a new key needs no balance.

## Errors

REST answers `{"error": "<code>", "msg": "<text>"}` with the HTTP status below. JSON-RPC answers
an error with the JSON-RPC code below and the same code in `data.error`.

| Code             | HTTP | JSON-RPC | When                                                    |
| ---------------- | ---- | -------- | ------------------------------------------------------- |
| `E_UNKNOWNUID`   | 400  | -32013   | no open challenge: missing, expired, used, or from another node |
| `E_BUSY`         | 503  | -32003   | too many open challenges                                |
| `E_EXPIRE`       | 400  | -32010   | `expire` out of range                                   |
| `E_EMPTYPUBLIC`  | 400  | -32010   | neither `pubkey` nor a registered `key_id`              |
| `E_DIFKEY`       | 400  | -32010   | `key_id` is not the address of the public key           |
| `E_SIGNATURE`    | 400  | -32010   | the signature does not verify                            |
| `E_NEWUSER`      | 401  | -32014   | the key is not registered in the ecosystem               |
| `E_ECONOTOPEN`   | 401  | -32014   | the ecosystem takes no new members                       |
| `E_DELETEDKEY`   | 403  | -32014   | the key is deleted                                       |
| `E_CHECKROLE`    | 403  | -32014   | the account is not a member of `role_id`                 |
| `E_KEYNOTFOUND`  | 404  | -32012   | guest login in an ecosystem without the guest account    |
