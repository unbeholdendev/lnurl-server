# lnurl-server

A small, self-hosted Lightning Address server written in Go. It lets you receive Lightning
payments at `you@yourdomain.com` using your own node, instead of relying on a custodial
provider.

Most Lightning Addresses are issued by a third party. That means someone else's server
creates the invoices you get paid with, and sees who is paying you. This serves them from
your own LND node instead.

## How it works

A Lightning Address is a human-readable wrapper around LNURL-pay:

1. A wallet resolves `you@yourdomain.com` to `GET /.well-known/lnurlp/you`
2. The server replies with a `payRequest` document: callback URL, min/max sendable amounts,
   and metadata
3. The wallet calls the callback with the amount it wants to send
4. The server asks LND to create an invoice (`AddInvoice` over gRPC) and returns the BOLT11
   string
5. The wallet pays the invoice and the funds settle in your node

## Features

- LNURL-pay / Lightning Address support (LUD-06, LUD-16)
- LND backend over gRPC with macaroon authentication
- YAML configuration
- Request rate limiting
- Docker image and Compose file for deployment
- CI via GitHub Actions

## Requirements

- A running LND node reachable over gRPC, plus its TLS certificate and a macaroon
- A domain with HTTPS terminated in front of the server (Caddy, nginx, or similar).
  The `.well-known` endpoint must be served over TLS for wallets to accept it.
- Go 1.25+ if building from source

## Quick start

```bash
git clone https://github.com/pycanis/lnurl-server.git
cd lnurl-server

cp config.example.yaml config.yaml
# edit config.yaml: your domain, address names, and LND connection details

docker compose up -d
```

Then verify it resolves:

```bash
curl https://yourdomain.com/.well-known/lnurlp/you
```

## Configuration

See [`config.example.yaml`](./config.example.yaml) for all available options, including the
LND host, TLS certificate and macaroon paths, the addresses to serve, and the sendable
amount limits.

## Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/.well-known/lnurlp/{name}` | Returns the LNURL-pay `payRequest` document |
| `GET` | *(callback from above)* | Returns a BOLT11 invoice for the requested amount |

## Security notes

Use an **invoice-only macaroon** rather than `admin.macaroon`. The server only ever needs to
create invoices, so it should not be able to move funds or read your channel state. You can
bake a restricted macaroon with `lncli bakemacaroon`.

The server never holds keys and never has spending authority — it asks your node for an
invoice and hands it to the payer.

## Development

```bash
go run ./cmd/lnurl-server
go test ./...
```

Layout follows the standard Go convention: entrypoint in `cmd/lnurl-server`, everything
else under `internal/`.

## References

- [LNURL specifications (LUDs)](https://github.com/lnurl/luds)
- [LUD-16: Lightning Address](https://github.com/lnurl/luds/blob/luds/16.md)
- [LND API reference](https://lightning.engineering/api-docs/api/lnd/)

## License

MIT
