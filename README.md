# lnproxy-address

A minimalist lightning address bridge

Host on a domain you own to get a lightning address for a node you own
with all invocies wrapped by lnproxy so that you don't ever reveal your
node's pubkey.

Configuration is 100% manual,
just make some json files in `user/username` and `node/username`.
Mine look like:
```
$ cat user/support
{
"UserName":"support",
"MaxAmtMsat":2000000000,
"MinAmtMsat":100000,
"NodeType":"LND"
}

$ cat node/support
{
"Host":"https://youronionhiddenservice.onion",
"Cert":"-----BEGIN CERTIFICATE-----\nyour\n.lnd/tls.cert\ngoes\nhere\n-----END CERTIFICATE-----",
"Macaroon":"000000000000000000000000000000000000000000000000000000000..."
}
```
Supports all the node types supported by https://github.com/fiatjaf/makeinvoice/
