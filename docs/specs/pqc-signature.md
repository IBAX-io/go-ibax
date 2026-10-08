# Post-quantum signatures (ML-DSA)

Status: draft, pending sign-off by the go-ibax maintainer and the Weaver lead.
Applies to: go-ibax (node) and Weaver (client).

This document fixes how go-ibax chains use ML-DSA (FIPS 204). Every rule is pinned by golden vectors or tests (see "Pinned by"); a change to a rule is a change to those vectors first.

## Notation

- `Hash` is the chain's hasher (`--hasher`): SHA256, KECCAK256, SHA3_256, SM3, and SHA384, SHA512 once they exist. `DoubleHash(x) = Hash(Hash(x))`.
- `Sign(key, data)` and `Verify(pub, data, sig)` are `packages/common/crypto.Sign` and `Verify`. Both apply `Hash` once to `data` before handing it to the signature algorithm.
- "The vectors" are the files in `tools/cryptovectors/testdata`; Weaver syncs them into `src/app/lib/crypto/fixtures`.

## Sizes

| Item | ML-DSA-65 | ML-DSA-87 |
|---|---|---|
| Private key (seed ξ) | 32 | 32 |
| Public key | 1,952 | 2,592 |
| Signature | 3,309 | 4,627 |
| Transfer transaction, measured (payload / signature section / total) | 2,112–2,131 / 3,312 / 5,428–5,447 | about 2,750 / 4,630 / 7,400 |
| ECDSA transfer, for comparison | 224–243 / 65 / 292–311 | |

The ML-DSA-65 and ECDSA rows come from `go-ibax-transfers.json`. The ML-DSA-87 row is derived from them (public key and signature swapped in); T9 replaces it with measured values.

## Rules

### 1. Algorithms and levels

`MLDSA65` (`AsymAlgo = 4`) and `MLDSA87` (`AsymAlgo = 5`, added with T9). A chain picks one with `--cryptoer`. National security systems use `MLDSA87` with `SHA384` or `SHA512` (CNSA 2.0); other federal chains may use either level.

Rationale: FIPS 204 approves both; CNSA 2.0 allows only ML-DSA-87.

Pinned by: the `cryptoer` field of the vectors; `cryptovectors` refuses a cryptoer go-ibax does not define.

### 2. What is signed

Pure ML-DSA (FIPS 204 Algorithm 2, `ML-DSA.Sign`) over the same digest every other algorithm signs:

- transaction: `Sign(key, DoubleHash(payload))`, so ML-DSA receives `Hash(DoubleHash(payload))`;
- login: `Sign(key, "LOGIN" + networkID + uid)`, so ML-DSA receives `Hash(login string)`;
- block: the node key signs the block's `ForSign` string the same way.

HashML-DSA (pre-hash mode) is not used.

Rationale: one signing path for all algorithms. The transaction hash is already used everywhere on chain. CNSA 2.0 does not allow HashML-DSA. PKCS#11 3.2 defines pure ML-DSA with a context parameter (`CKM_ML_DSA`), so an HSM produces the same signature (rule 6).

Pinned by: `goSignature` (node, over `message`) and `clientSignature` (client, over `DoubleHash(payload)`) in `go-ibax-vectors.json`; the signed transactions in `go-ibax-transfers.json`.

### 3. Context string

The FIPS 204 context is `IBAX-MLDSA-65-v1` for ML-DSA-65 and `IBAX-MLDSA-87-v1` for ML-DSA-87. Login and transactions share it. A signature under any other context, including the empty one, is refused.

Rationale: the context separates IBAX signatures from ML-DSA signatures the same key might make elsewhere. Login and transactions need no separate contexts: the login string starts with `LOGIN` and a transaction payload is msgpack, so the two preimages cannot collide, and both are hashed before signing.

Pinned by: `contextFreeSignature` in `go-ibax-vectors.json`, a valid empty-context signature of the same digest that the node (`cryptovectors check`) and Weaver (`suites.test.ts`) must refuse.

### 4. Randomness

Hedged signing (FIPS 204 default): every signature mixes fresh randomness with the key. Deterministic ML-DSA is not used. Signatures of the same digest therefore differ, and vectors are compared by verifying, not byte for byte.

Rationale: hedged signing resists fault and side-channel attacks better than the deterministic variant, and matches the hedged nonces go-ibax uses for ECDSA and SM2.

Pinned by: `TestNodeSignaturesAreHedged` (go-ibax); Weaver `suites.test.ts` ("signs ML-DSA-65 hedged").

### 5. Key ID and address

The same rule as every other algorithm: `KeyID = crc64(SHA512(Hash(publicKey)))`, formatted as an IBAX address. The full 1,952- or 2,592-byte public key is hashed; nothing is stripped (`CutPub` only strips the `04` prefix of a 65-byte curve key).

Rationale: one address rule for all algorithms. A shorter digest of the public key, such as SHA256(publicKey)[:20], would be a second address format with no gain: the address is an int64 either way.

Pinned by: `keyID` in `go-ibax-vectors.json`; `go-ibax-addresses.json`.

### 6. Keys

- Non-FIPS chains: the wallet's 32-byte private key is the FIPS 204 seed ξ (`ML-DSA.KeyGen_internal(ξ)`). An existing wallet therefore has a fixed ML-DSA address on every ML-DSA chain, with no new mnemonic.
- FIPS chains: the key is generated and kept inside an HSM or hardware token (PKCS#11) and never leaves it. The client's software signer refuses to sign (T11).

Rationale: a wallet keeps one secret across all algorithms. FIPS 140-3 requires signing inside a validated module, which also rules out deriving the key in software.

Pinned by: `publicKey` in `go-ibax-vectors.json` for three seeds, including the all-zero seed; the public-key fingerprint test in `asymalgo/mldsa65_test.go`.

### 7. Transaction format

The transaction format is unchanged and has no version bump. The public key travels in the header (`Header.PublicKey`); the signature section uses the existing multi-byte length prefix (`converter.EncodeLength`, three bytes for 3,309 and 4,627). Clients and SDKs must implement multi-byte lengths; a one-byte length is not enough.

Rationale: the length prefix already supports any size. A version bump would split clients with no change in meaning.

Pinned by: the ML-DSA-65 transfers in `go-ibax-transfers.json` (3,309-byte signature); the ML-DSA-87 transfers added with T9 (4,627 bytes).

### 8. Storage

Public keys are stored as they are, at full length. Every column holding a public key is `bytea`, or `text` for hex, without a length limit. `1_keys.pub` already is. `1_candidate_node_requests.node_pub_key` is created outside this repository; T9 checks its definition and widens it if needed.

Rationale: a truncated or rejected public key makes an account or node unusable on an ML-DSA chain.

Pinned by: the Weaver chain e2e (`suites.chain`): on every ML-DSA suite a new account registers, logs in and transfers, so the full key is written to and read from `1_keys`.

### 9. Size limits and fees

The transaction size limit, the block size accounting and the storage fee all count the full serialized transaction: type byte, payload and signature section.

Today go-ibax counts only the payload (`txMaxSize.check` in `transaction/limits.go`, and `TxSize = len(Payload)` for `storageFeeBy` in `smart/gas.go`). The signature section is not counted, while a receiving node checks the full block bytes against `max_block_size` (`block/db.go`). The gap is 65 bytes per ECDSA transaction and 3,312 (ML-DSA-65) or 4,630 (ML-DSA-87) bytes per ML-DSA transaction. A producer can therefore build a block that its own accounting accepts and its peers refuse. Fixing this changes fees on every chain, so all nodes upgrade together; it is implemented with T9.

With the defaults (`max_tx_size` 32 MiB, `max_block_size` 64 MiB, `max_tx_block` 5,000, `price_tx_size` 15), no parameter has to change:

| | ECDSA | ML-DSA-65 | ML-DSA-87 |
|---|---|---|---|
| Transfer, full bytes | 292 | 5,428 | about 7,400 |
| Storage fee per transfer (`15 × bytes / 2^20` tokens) | 0.0042 | 0.0776 | about 0.106 |
| 5,000 transfers per block | 1.5 MB | 27 MB | 37 MB, under 64 MiB |

Signature verification is not charged as fuel for any algorithm. ML-DSA verification costs about as much CPU time as ECDSA verification, so it needs no charge either.

Rationale: an ML-DSA signature is 61% of the transaction. Limits and fees that skip it neither bound the block nor price the storage.

Pinned by: the sizes in `go-ibax-transfers.json`; T9 adds a test that a block filled to the limit is accepted by its peers.

### 10. Implementations

- go-ibax: the Go standard library `crypto/mldsa` (Go 1.27). On FIPS chains it runs inside the Go Cryptographic Module (T10).
- Weaver: `@noble/post-quantum` on non-FIPS chains; on FIPS chains, a validated module through PKCS#11 (T11).

Rationale: FIPS 140-3 validates modules, not algorithms. The Go module and an HSM can be validated; `@noble/*` and `cloudflare/circl` are not.

Pinned by: the vectors themselves. The node verifies every `clientSignature` (made by `@noble/post-quantum`); Weaver verifies every `goSignature` (made by `crypto/mldsa`).

### 11. Migration

A wallet keeps one identity per suite (`identities[suiteKey]` in Weaver's keyring), so the same secret has a separate address on every chain. The client checks the chain's suite when it restores a session, logs in, and when a transaction is refused; if the suite changed, it signs out with the reason. A chain cannot change its algorithm in place: changing `--cryptoer` or `--hasher` on an existing chain stops block production, so a new algorithm means a new genesis.

Rationale: tested on a local chain (see the crypto alignment task document). An in-place switch leaves the chain unable to produce blocks.

Pinned by: one private key across all suites in `go-ibax-vectors.json`, each with its own `keyID`; the Weaver chain e2e `session.chain` (suite changes in place).

## Superseded ibax-federal decisions

`ibax-federal/federal/docs/mldsa-spec.md` and `compliance/roadmap.md` recorded four decisions. This document replaces them:

| ibax-federal | Here | Why |
|---|---|---|
| Sign `SHA256(data)` | Sign `Hash(data)` with the chain's hasher (rule 2) | One signing path; SHA256 is one of the hashers |
| Context `IBAX-MLDSA-65-v1` | Kept, plus `IBAX-MLDSA-87-v1` (rule 3) | |
| KeyID `SHA256(publicKey)[:20]` | The chain's address rule (rule 5) | One address format |
| Key from BIP39 → HKDF-SHA256 | Wallet private key as seed on non-FIPS chains; HSM on FIPS chains (rule 6) | No second derivation; FIPS chains keep keys in a module |
| Parallel batch verification (`mldsa_batch.go`) | Not adopted | Every algorithm verifies through the same per-transaction path |
| BoringCrypto as the FIPS module | The Go Cryptographic Module (`GOFIPS140`), T10 | Go 1.24+ ships its own module |

Still valid from the roadmap, and kept here as rules for all documentation:

- FIPS 140-3 validates a cryptographic module, not an algorithm. ML-DSA being a FIPS standard does not make an implementation validated.
- No material may claim a validation that has not been granted. The Go Cryptographic Module v1.0.0 is validated (CMVP #5247) and has no ML-DSA; v1.26.0, which has ML-DSA, is in process. Whether in-process modules are acceptable is a compliance decision (T10).

The SP 800-53 control mapping in the roadmap belongs to the SIEM work (T12), not to this document.

## Sign-off

| Role | Name | Date |
|---|---|---|
| go-ibax maintainer | | |
| Weaver lead | | |
