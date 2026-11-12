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

4. Set up PostgreSQL for the node, with these settings in its `postgresql.conf`:

```ini
fsync = on
max_connections = 1000
max_wal_size = 32GB
checkpoint_timeout = 15min
wal_compression = zstd
shared_buffers = 8GB          # a quarter of the memory of the server, up to 8 GB
backend_flush_after = 256kB
checkpoint_flush_after = 256kB
bgwriter_flush_after = 512kB
```

   With the defaults (`max_wal_size` 1 GB, `shared_buffers` 128 MB) a node under load runs a checkpoint every
   few seconds and most of its WAL is full page images: on 5 nodes of one server at 200 transactions/s, the
   settings above cut the WAL from 34.7 to 11.2 GB and the disk writes from 80.1 to 19.4 GB in 15 minutes.
   Keep 32 GB free for the WAL. `max_connections` covers the connections of the node (`DB.MaxOpenConns`) and of
   the other nodes the server holds. dapps `tools/localchain/postgresql.conf` is the same profile.

   The server and the disk of a validator:

   - Small dirty page limits, in bytes, so the kernel writes back all the time instead of in bursts that
     hold every synchronous write of the disk behind them (`/etc/sysctl.d/90-ibax.conf`, then `sysctl --system`):

     ```ini
     vm.dirty_background_bytes = 67108864
     vm.dirty_bytes = 268435456
     ```

     The `*_flush_after` settings above do the same for PostgreSQL.
   - A disk with a DRAM cache or power loss protection (a data center NVMe). A validator stores its sign state
     with a synchronous write before it sends a message; when the disk stops writing, the validators on it stop
     voting. A consumer NVMe without DRAM stopped all writes for 4-8 s, every 5-10 minutes, while a node joined
     under a load of 200 transactions/s: P99 14.3 s, 35 propose timeouts; the same test with the data in memory
     had P99 1.2 s and no propose timeout.
   - The check: join a node from a snapshot under the load of the network for 40 minutes. The disk passes when
     no validator on it logs `storing the sign state held the messages` with `took` of 1 s or more.

5. Initialize the database.

```bash
$    go-ibax initDatabase
```

6. Start go-ibax.

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
