# cryptovectors

Golden vectors for the cryptography and transaction decoding shared by the client (Weaver) and the node. Each JSON file holds inputs and what the node computes from them; every node-side field is recomputed by this tool with go-ibax's own code.

| File | Inputs (written by) | Fields computed by the node |
|---|---|---|
| `go-ibax-vectors.json` | cryptoer, hasher, privateKey, message, payload (this repository); clientSignature (client) | publicKey, keyID, goSignature; clientSignature must pass the node's verification |
| `go-ibax-addresses.json` | input (this repository) | id, address |
| `go-ibax-transfers.json` | transaction parameters and data (client) | node: result of decoding, Validate and CheckSign |
| `go-ibax-contract-params.json` | declared types and data (client) | header, node: FillTxData result per parameter |

```sh
go run ./tools/cryptovectors gen     # recompute the node-side fields and write them back
go run ./tools/cryptovectors check   # compare only; exits non-zero on any difference
go test ./tools/cryptovectors/...    # what CI runs: check, plus tampered vectors that must fail
```

ECDSA and SM2 signatures are randomized: a `goSignature` is kept as long as it still verifies and is not compared byte for byte.

When the client changes the transaction encoding or adds cases, regenerate data / clientSignature in the client repository, write them here, then run `gen`; the client repository then syncs the four files back.
