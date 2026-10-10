# Notifications

The node pushes the counts of an account's open notifications to its clients through
[Centrifugo](https://centrifugal.dev) 5 or later. It publishes with the server HTTP API of
Centrifugo, and the session of every login carries a connection token for it.

## The channel of an account

The notifications of an account are published to the channel `client#<account>`, `<account>`
being its address (`1234-5678-9012-3456-7890`). The token of a session, `notify_key` in the answer
of login, subscribes its connection to this channel on the server side:

```json
{ "sub": "1234-5678-9012-3456-7890", "channels": ["client#1234-5678-9012-3456-7890"], "exp": 1760028800 }
```

It is signed with HMAC-SHA256 by the secret `--centSecret`, and expires with the session. A client
subscribes to no channel: it gets the publications of its account's channel on its connection. A
connection cannot subscribe to the channel of another account, nor connect after its token
expires.

A publication is the list of the account's counts in the ecosystem of the change, a role 0 for the
notifications sent to the account itself:

```json
[{ "ecosystem": "1", "role_id": "0", "count": 3 }, { "ecosystem": "1", "role_id": "2", "count": 1 }]
```

## Endpoint

| REST                    | JSON-RPC                        |
| ----------------------- | ------------------------------- |
| `GET config/centrifugo` | `ibax.getConfig("centrifugo")`  |

```json
{ "url": "ws://127.0.0.1:8000", "protocol": "centrifuge-json", "version": "6" }
```

`url` is `--centUrl` with the WebSocket scheme; clients connect to `<url>/connection/websocket`.
`version` is the major version of the server, which the node asks it each time. When Centrifugo
does not answer, REST answers `503` with the error `E_CENTRIFUGO`, JSON-RPC the error `-32003`.

## Running Centrifugo

The node needs the server's address (`--centUrl`), its HTTP API key (`--centKey`) and its token
secret (`--centSecret`). The configuration of Centrifugo 6 they match:

```json
{
  "client": {
    "token": { "hmac_secret_key": "<--centSecret>" },
    "allowed_origins": ["https://<the origin of the client>"]
  },
  "http_api": { "key": "<--centKey>" }
}
```

Nothing else is needed: the subscriptions are on the server side, and channels without a namespace
take no subscription from clients.
