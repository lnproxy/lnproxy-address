# lnproxy-address

A minimalist lightning address bridge.

Host on a domain you own to get a lightning address for a node you own
with all invocies wrapped by lnproxy so that you don't ever reveal your
node's pubkey.
Automatically checks the payment hashes, amount, and destination, on wrapped invoices,
so you get privacy without trusting anyone with your funds.

```
usage: address [flags] address.macaroon https://example.com
  address.macaroon
        Path to address macaroon. Generate it with:
                lncli bakemacaroon --save_to address.macaroon \
                        uri:/invoicesrpc.Invoices/AddInvoice
  https://example.com
        Your custom domain
  -lnd string
        host for lnd's REST api (default "https://127.0.0.1:8080")
  -lnd-cert string
        lnd's self-signed cert (set to empty string for no-rest-tls=true) (default ".lnd/tls.cert")
  -lnproxy-routing-base uint
        base routing budget for lnproxy relay (default 2000)
  -lnproxy-routing-ppm uint
        ppm routing budget for lnproxy relay (default 10000)
  -lnproxy-url string
        REST API url for lnproxy relay, empty string for no proxying (default "https://lnproxy.org/spec/")
  -max-msat uint
        max msat per payment (default 10000000000)
  -min-msat uint
        min msat per payment (default 10000)
  -port string
        http port over which to expose api (default "4747")
  -username string
        lud6 username (default "_")
```
