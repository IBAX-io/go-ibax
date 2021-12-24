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

3. Genereate the first block. If you are creating your own blockchain network. you must use the `--test=true` option.
   Otherwise you will not be able to create new accounts.

```bash
$    go-ibax generateFirstBlock \
        --test=true
```

4. Initialize the database.

```bash
$    go-ibax initDatabase
```

5.Starting go-ibax.

```bash
$    go-ibax start
```



