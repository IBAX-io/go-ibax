# IBAX Blockchain System Platform

[![Go Reference](https://pkg.go.dev/badge/github.com/IBAX-io/go-ibax.svg)](https://pkg.go.dev/github.com/IBAX-io/go-ibax)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBAX-io/go-ibax)](https://goreportcard.com/report/github.com/IBAX-io/go-ibax)

## The Most Powerful Infrastructure for Applications on Decentralized/Centralized Ecosystems

A powerful blockchain system platform with a new system framework and a simplified programming language, it is including
smart contract, database table and interface.

### Build from Source

#### Install Go

The build process for go-ibax requires Go 1.17 or higher. If you don't have it: [Download Go 1.17+](https://go.dev).

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
