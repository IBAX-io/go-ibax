# IBAX Blockchain System Platform

[![Go Reference](https://pkg.go.dev/badge/github.com/IBAX-io/go-ibax.svg)](https://pkg.go.dev/github.com/IBAX-io/go-ibax)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBAX-io/go-ibax)](https://goreportcard.com/report/github.com/IBAX-io/go-ibax)

## The Most Powerful Infrastructure for Applications on Decentralized/Centralized Ecosystems

A powerful blockchain system platform with a new system framework and a simplified programming language, it is including
smart contract, database table and interface.

### Branches and chains

A binary of the `develop` branch runs **newly created chains only**: chains whose first block a
`develop` binary generated. It must not upgrade any existing chain in place, including the
production nodes that run the `main` release line (v1.4.x) on mainnet and testnet: those nodes
keep the binaries of `main`.

Nor does a chain of `develop` take, in place, a later `develop` binary whose consensus rules or
first-block parameters differ from those of the binary that generated its first block. The merge
of `feat/tps` is such a change: it raises the starting `bft_timeout_propose_ms` from 3000 to 4000
and bounds the timeouts by margins. A chain created before that merge keeps its binaries; a chain
on the merged code is created anew by a binary of the merged code.

### Build from Source

#### Install Go

The build process for go-ibax requires Go 1.27 or higher. If you don't have it: [Download Go 1.27+](https://go.dev).

You'll need to add Go's bin directories to your `$PATH` environment variable e.g., by adding these lines to
your `/etc/profile` (for a system-wide installation) or `$HOME/.profile`:

```
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$GOPATH/bin
```

(If you run into trouble, see the [Go install instructions](https://go.dev/dl/)).

#### Compile

```
$ export GOPROXY=https://athens.azurefd.net
$ GO111MODULE=on go mod tidy -v

$ go build
```

### Run

1. Create the node configuration file:

```bash
$    go-ibax config
```

2. Generate node keys:

```bash
$    go-ibax generateKeys
```

3. Generate the first block. If you are creating your own blockchain network. You must use the `--test=true` option.
   Otherwise you will not be able to create new accounts.

```bash
$    go-ibax generateFirstBlock --test=true
```

4. Initialize the database.

```bash
$    go-ibax initDatabase
```

5.Starting go-ibax.

```bash
$    go-ibax start
```




## Run Modes

`RunNodeMode` in the config selects how the node operates:

| Mode | Value | Description |
|------|-------|-------------|
| Node | `NONE` | Regular blockchain node (default) |
| ChainHost | `ChainHost` | Manages isolated child-chain instances (BaaS-style) |
| ChildChain | `ChildChain` | Runs as an isolated child chain |
| SubNode | `SubNode` | Ecosystem-level node syncing only its own ecosystem's data |

> **Naming note (2026-10):** `ChainHost`/`ChildChain` were renamed from
> `CLBMaster`/`CLB`. The old names collided with **CLB (Cross Ledger Base)**,
> the cross-ledger communication protocol implemented by the separate
> `go-ibax-clb` repository (whitepaper §6.2.3). The run modes only provision
> isolated chain instances and implement no cross-ledger function.
> Old config values are not accepted; update `RunNodeMode` accordingly.
> (Pre-mainnet: no migration needed for deployed networks.)
