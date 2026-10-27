# sign-customjson

Sign and broadcast a Hive transaction with a Ledger hardware wallet. Adapted
from go-vsc-node's `cmd/contract-deployer` Ledger signing harness.

Two modes:

- **custom_json** (`-id` / `-json`) — you give the payload and the `custom_json`
  operation is built for you.
- **operations** (`-ops` / `-ops-file`) — you give the operations, whole, as
  JSON. Any operation type works.

Either way the transaction around the operations — reference block and
expiration — is filled in from the chain head, so you never deal with TaPoS.

## How it works

In both modes the Go tool computes the 32-byte signing digest, then hands it to
an external signing command on stdin. The bundled [ledger-signer](./ledger-signer/)
asks the Ledger Hive app to sign that exact hash (hash/blind signing) and prints
the signature on stdout. The Go tool attaches it and broadcasts.

custom_json mode serializes with **hivego** — the same serializer used to
broadcast — so the signed bytes and the broadcast bytes can never diverge.

Operations mode instead asks the Hive nodes themselves to serialize the
transaction (`get_transaction_hex`), which is what lets it handle any operation
type without this tool knowing anything about it. That does mean the bytes being
signed come from a node, and since the Ledger signs the digest blind (it shows
only a hash) a lying endpoint could substitute a different transaction without it
being visible on the device. So **every endpoint listed in `-api` is asked and
all of them must agree** — list two or three independent ones and a single
compromised node becomes a hard failure instead of a signed transaction.

## Setup (once)

```bash
cd apps/sign-customjson/ledger-signer
pnpm install          # installs deps and builds -> dist/sign.js
```

On the device: unlock it, open the **Hive** app, and **enable hash signing**
in the app settings (*Settings → Sign raw hashes → Enabled*). See the
[ledger-signer README](./ledger-signer/README.md) for prerequisites (udev
rules on Linux, native build toolchain) and troubleshooting.

Confirm the connection and find your key's derivation path:

```bash
pnpm -s -C apps/sign-customjson/ledger-signer run pubkey
pnpm -s -C apps/sign-customjson/ledger-signer run findpath -- STM<yourKey>
```

## Use: custom_json

From the `go/` root (the default `-ledger-sign-cmd` assumes this cwd):

```bash
go run ./apps/sign-customjson \
  -account myaccount \
  -id vsc.call \
  -json '{"net_id":"vsc-mainnet","action":"..."}' \
  -ledger-path "m/48'/13'/1'/0'/0'"
```

- `-json-file payload.json` (or `-json-file -` for stdin) instead of `-json`.
- `-posting` authorizes with the posting authority instead of active — point
  `-ledger-path` at the matching key (use `findpath` above to locate it).

## Use: any other operation

Write just the operations — the same shape CLIVE and the `database_api` docs
use — and pass them with `-ops-file` (or `-ops` inline, or `-ops-file -` for
stdin):

```json
{
  "operations": [
    {
      "type": "account_update2_operation",
      "value": {
        "account": "acc0",
        "active": {
          "account_auths": [["acc1", 1], ["acc2", 1], ["acc3", 1]],
          "key_auths": [],
          "weight_threshold": 2
        },
        "extensions": [],
        "json_metadata": "",
        "posting_json_metadata": ""
      }
    }
  ]
}
```

```bash
go run ./apps/sign-customjson \
  -ops-file txs/multisig_active.json \
  -api https://api.hive.blog,https://api.deathwing.me \
  -ledger-path "m/48'/13'/1'/0'/0'"
```

`ref_block_num`, `ref_block_prefix`, `expiration` and `extensions` are filled in
from the chain head. A bare `[...]` array works as well as the `{"operations":
[...]}` wrapper, and anything else alongside `operations` is ignored — a
transaction pasted in whole gets fresh TaPoS regardless.

Beyond that, the operations are passed to the node exactly as written. Every
field the operation needs must be there; a malformed or incomplete one comes back
as the node's own error, not this tool's.

### Operation encodings

Hive has two, and this tool follows whichever you use:

| Written as | Example | Assets | API used |
| --- | --- | --- | --- |
| typed | `{"type": "transfer_operation", "value": {...}}` | NAI objects | `database_api` / `network_broadcast_api` |
| legacy | `["transfer", {...}]` | `"1.000 HIVE"` | `condenser_api` |

They are not interchangeable — each API rejects the other's form — so all the
operations in one run have to be written the same way. Mixing them is an error.

## Common options

- `-no-broadcast` signs on the device but prints the signed transaction JSON
  instead of broadcasting; `-out signed.json` also writes it to a file.
- `-api` comma-separated Hive RPC endpoints (default `https://api.hive.blog`).
  In operations mode all of them must agree on the serialized transaction (see
  above).
- `-expiration` seconds until the transaction expires (default 1800, max 3600).
- `-chain-id` hex chain id for testnets (empty = Hive mainnet).
- `-account` the account authorizing the operation. Required for custom_json;
  in operations mode it is optional and only used for display.
- `-ledger-sign-cmd` any command that reads the signing request JSON on stdin
  and prints the signature hex on stdout, if you don't want the bundled signer.
