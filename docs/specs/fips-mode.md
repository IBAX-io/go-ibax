# FIPS 140-3 mode

Applies to: go-ibax (node), and the clients that sign for a FIPS chain (see "Clients").

A node runs in FIPS 140-3 mode when every cryptographic service it relies on comes from a validated module, used only in approved ways. go-ibax gets this from the Go Cryptographic Module (Go 1.24 and later): the node is built against a frozen version of the module and started with FIPS mode on.

## Build and run

```sh
GOFIPS140=v1.26.0 go build -o go-ibax .     # or v1.0.0-c2097c7c, see "Module versions"
GODEBUG=fips140=on ./go-ibax start ...      # fips140=only also fails every non-approved call
```

`GOFIPS140` selects the module version compiled in and turns FIPS mode on by default; `GODEBUG=fips140=on` (or `only`) states it explicitly at run time. `GODEBUG=fips140=off` on a `GOFIPS140` build turns it off again, so the run-time setting is what counts.

The node reports the mode as `fips` (bool) in `GET /api/v2/getuid` and in the JSON-RPC `getUid` result, next to `cryptoer` and `hasher`.

## Module versions

| `GOFIPS140` | Status | ML-DSA |
|---|---|---|
| `v1.0.0-c2097c7c` (alias `certified`) | CMVP certificate | no: `crypto/mldsa` fails on every key |
| `v1.26.0` (alias `inprocess`) | in the CMVP process | yes |

A chain that signs with ML-DSA needs `v1.26.0` or a later module. The node checks this at startup in every mode: a node built with `v1.0.0` refuses to start with `MLDSA65` or `MLDSA87` and says which module it was built with.

## Approved suites

In FIPS mode the node starts only with:

| Setting | Approved |
|---|---|
| `--cryptoer` | `ECC_P256`, `MLDSA65`, `MLDSA87` |
| `--hasher` | `SHA256`, `SHA384`, `SHA512`, `SHA3_256` |

`ECC_Secp256k1`, `SM2`, `SM3` and `KECCAK256` are not FIPS algorithms (Keccak-256 is not FIPS 202 SHA3-256); a node in FIPS mode configured with one of them exits at startup and logs why. That gives 12 suites with `v1.26.0` and 4 (`ECC_P256` with each hasher) with `v1.0.0`.

FIPS mode is a property of the chain, not of one node: every node of a chain hashes and signs with the same suite, and a FIPS chain must run every node in FIPS mode. A non-FIPS node on the same suite computes the same hashes and signatures, so nothing on the wire tells them apart; the operator is responsible for starting all nodes with the same build and `GODEBUG`.

## Genesis and configuration

At startup the node checks that the genesis block was made with its configured suite, before it loads the first block (from the file or from a peer) and on every later start:

1. the prev hashes of the genesis header are `DoubleHash("0")` and `Hash("0")` under the configured hasher;
2. the merkle root and the block hash recompute under the configured hasher;
3. the header signature verifies under the configured cryptoer with the node key in the genesis transaction.

A mismatch stops the node with "the genesis block does not match the configured crypto suite". This applies in every mode: a node on another suite would fork from the first block it makes.

## What else FIPS mode changes

- HMAC keys shorter than 112 bits (14 bytes) are refused, as SP 800-131A requires: the `HMac` contract function returns an error, and the node exits at startup when the Centrifugo secret (`--centSecret`, which signs Centrifugo tokens with HMAC-SHA256) is shorter; the default `127.0.0.1` is. Outside FIPS mode any key length works.
- `SHA3_256` comes from `crypto/sha3` (inside the module) instead of `golang.org/x/crypto/sha3`.
- Links to binaries and to long text or blob values (`/data/...`) carry the network hash or SHA-256; MD5 is no longer used, in the node or in the SQL it runs.
- The HTTPS API follows SP 800-52 Rev. 2: TLS 1.3, or TLS 1.2 with ECDHE and AES-GCM only. In FIPS mode Go further restricts both to approved groups and signature schemes.

## Clients

FIPS mode covers what the node does. Transactions and logins are signed by the client, and a FIPS chain needs those signatures from a validated module too: the node cannot tell, so the client decides by the `fips` flag of `getuid`. Weaver signs only with a key in a PKCS#11 module (an HSM or a token) on a network that reports `fips: true`; that is the desktop app, and the web app only reads there.

Signing in as the guest takes no signature: `POST /api/v2/login` with `guest=true` (JSON-RPC `login` with `"guest": true`) opens a session of the guest account (`consts.GuestKey`) in the ecosystem asked for, and ignores `pubkey`, `key_id`, `signature` and `role_id`. The guest key is public, so a signature with it proved nothing. Clients use it to read a network without signing in software; every node accepts it, in every mode.

## Outside the boundary

Code that does not use the module for a security function, and is not affected by FIPS mode:

- the `EthereumAddress` and `Bitcoin*Address` contract functions encode other chains' addresses (Keccak-256 with the EIP-55 checksum, RIPEMD-160, Bech32) with `golang.org/x/crypto` and btcutil; they derive identifiers, they do not protect anything;
- the node-to-node TCP protocol is not encrypted; blocks and transactions it carries are authenticated by their signatures.

## Pinned by

- `packages/common/crypto/fips_test.go`: which algorithms and HMAC key lengths each mode and module accepts; a node in FIPS mode exits on every other one; every approved suite signs, verifies, hashes and MACs under `fips140=only`.
- `packages/block/genesis_test.go`: a genesis made under one suite is refused under every other one.
- `packages/chain/tls_test.go`: which TLS versions and suites the API accepts.
- `.github/workflows/crypto.yml`, job `fips`: the tests above built against both modules and run with `GODEBUG=fips140=only`.
- Weaver `e2e/chain` with `CHAIN_E2E_FIPS`: a 3-node chain on every approved suite, built against a module and run with `fips140=only`.
