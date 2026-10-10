# cryptovectors

Golden vectors for the cryptography and transaction decoding shared by the client (Weaver) and the node. Each JSON file holds inputs and what the node computes from them; every node-side field is recomputed by this tool with go-ibax's own code.

| File | Inputs (written by) | Fields computed by the node |
|---|---|---|
| `go-ibax-vectors.json` | cryptoer, hasher, privateKey, message, payload (this repository); clientSignature (client) | publicKey, keyID, goSignature; ML-DSA only: contextFreeSignature, signed under the empty context, which the node and the client must refuse; clientSignature must pass the node's verification |
| `go-ibax-addresses.json` | input (this repository) | id, address |
| `go-ibax-transfers.json` | transaction parameters and data (client) | node: result of decoding, Validate and CheckSign |
| `go-ibax-account-keys.json` | derived from the SHA256 vectors of `go-ibax-vectors.json` (this tool) | node: accepted, or the error: each key, the key relabelled as every other account algorithm, and malformed encodings (bare, SEC1, unknown or unimplemented algorithm, a byte short or more); a client must refuse what the node refuses |
| `go-ibax-contract-params.json` | declared types and data (client) | header, node: FillTxData result per parameter |

```sh
go run ./tools/cryptovectors gen     # recompute the node-side fields and write them back
go run ./tools/cryptovectors check   # compare only; exits non-zero on any difference
go test ./tools/cryptovectors/...    # what CI runs: check, plus tampered vectors that must fail
```

ECDSA, SM2 and ML-DSA signatures are randomized: a `goSignature` is kept as long as it still verifies and is not compared byte for byte.

When the client changes the transaction encoding, signing or the cases, run one round in this order (the client's case list and its rules for changing data are in Weaver `src/app/test/cryptovectors.ts`):

```sh
# 1. Weaver: write the client fields (data, hash, clientSignature, parameter types/data) into this directory
GO_IBAX_DIR=/path/to/go-ibax npm run vectors:client
# 2. go-ibax: fill in the node fields and commit
go run ./tools/cryptovectors gen && go test ./tools/cryptovectors/... && git commit
# 3. Weaver: sync the five files back and record the go-ibax commit (src/app/lib/crypto/fixtures/go-ibax-commit.txt)
GO_IBAX_DIR=/path/to/go-ibax npm run vectors:sync && npm test
```

Coverage (checked by the tests on both sides): all 30 suites (5 cryptoers × 6 hashers) have a transaction the node accepts in transfers; all 90 signature vectors carry a clientSignature that the node verifies; all 36 ML-DSA vectors (ML-DSA-65 and ML-DSA-87) carry a contextFreeSignature that the node refuses. The rules these vectors pin down are in [docs/specs/pqc-signature.md](../../docs/specs/pqc-signature.md).
