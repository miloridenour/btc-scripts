# hive-customjson-ledger-signer

A small, **self-contained** TypeScript helper that signs a Hive custom_json
transaction with a Ledger hardware wallet. It exists only for the
`sign-customjson` app's Ledger flow.

It is **not** part of the Go build; nobody who doesn't use the Ledger flow
needs Node installed. Everything it needs lives in this directory;
`node_modules/` and `dist/` are git-ignored.

## What it does

It plugs into `sign-customjson`'s `-ledger-sign-cmd` hook. The Go tool builds
the transaction, computes the signing digest with **hivego** (the same
serializer used to broadcast, NAI/HF26 asset format), and hands this script
the digest on stdin. This script asks the Ledger to sign that exact 32-byte
hash and prints the signature on stdout. The Go tool then attaches it and
broadcasts.

Under the hood it uses [`@aioha/ledger-app-hive`](https://www.npmjs.com/package/@aioha/ledger-app-hive)
over a USB-HID transport, calling the Hive app's **hash-signing** path
(`signHash`).

> **Why hash signing.** The device signs the digest Go already computed — nothing
> in this script serializes or re-hashes the transaction. That is deliberate:
> there is exactly one serializer (hivego) in the whole flow, so the bytes the
> node broadcasts and the bytes the device signs can never diverge. Re-serializing
> the tx in JS and comparing digests fails because JS Hive libraries serialize
> assets in the legacy format while hivego uses NAI — a mismatch that aborts
> before the device is ever prompted.
>
> The tradeoff is that hash signing is **blind**: the Ledger shows only the hash,
> not a decoded transaction. You must enable it on the device (see Setup).

## I/O contract

- **stdin**: the signing-bundle JSON. Only two fields are read: `signing_digest`
  (64 hex chars / 32 bytes) and `path` (BIP32). Everything else is ignored.
- **stdout**: only the signature hex (so the caller parses it cleanly).
- **stderr**: prompts and diagnostics.
- **exit code**: non-zero on any failure (missing deps, no device, hash signing
  disabled, rejection on device).

This script does **no** serialization and **no** digest verification — it signs
the digest it is given.

## Toolchain

This package uses **pnpm** (the `packageManager` field records the version it was
set up with; `engines` requires `pnpm >=9`). Any pnpm you already have installed
works — you don't need Corepack. If you don't have pnpm: `npm install -g pnpm`.

The source is TypeScript (`sign.ts`) compiled to `dist/sign.js` with `tsc`. The
`prepare` script builds automatically on install, so a single `pnpm install`
gives you a runnable `dist/sign.js`.

## Setup (once)

Prerequisites:

- Node matching [`.nvmrc`](./.nvmrc) (`nvm use` / `fnm use`). Building the
  `node-hid` native module needs a C toolchain + libusb/hidapi headers.
- Linux: install the [LedgerHQ udev rules](https://github.com/LedgerHQ/udev-rules)
  so a non-root user can reach the device.
- On the device: unlock it and open the **Hive** app, then **enable hash
  signing** in the Hive app settings (e.g. *Settings → Sign raw hashes /
  Hash signing → Enabled*). This signer always uses the hash-signing path, so it
  is required — not optional.

Install and build:

```bash
cd apps/sign-customjson/ledger-signer
pnpm install          # installs deps and runs `prepare` (tsc) -> dist/sign.js
```

> **pnpm blocks dependency build scripts by default.** `node-hid` needs its
> native build to run, so it's whitelisted via `pnpm.onlyBuiltDependencies` /
> the workspace `allowBuilds`. If pnpm still prompts, run `pnpm approve-builds`
> and allow `node-hid`.

### Troubleshooting: Corepack signature errors

Corepack is **optional** here. If you've run `corepack enable` and `pnpm install`
then fails with `Error: Cannot find matching keyid`, that's a known Corepack bug:
the Corepack bundled with your Node ships outdated npm-registry signing keys, so
its signature check rejects current pnpm releases. Fixes:

- Skip Corepack and use your installed pnpm directly: `corepack disable`.
- Or update Corepack (newer versions have current keys): `npm install -g corepack@latest`.
- Or, on Corepack ≥ 0.31, bypass the check: `COREPACK_INTEGRITY_KEYS=0 pnpm install`.

## Test the connection first

Before signing for real, confirm the cable, app, path, and hash-signing setting
with the bundled `pubkey` helper. It reads the Hive app name/version, whether
hash signing is enabled, and the public key at a path — exercising the same
transport the signer uses, without building or signing a transaction:

```bash
# default path m/48'/13'/1'/0'/0', no device confirmation:
pnpm -s -C apps/sign-customjson/ledger-signer run pubkey

# a specific path, and confirm the address on the device screen:
pnpm -s -C apps/sign-customjson/ledger-signer run pubkey -- "m/48'/13'/0'/0'/0'" --confirm
```

Expected stderr looks like `device app: Hive v…` and `hash signing: ENABLED`,
with the `STM…` key on stdout. If you see `Locked device (0x5515)`, unlock the
Ledger and open the Hive app. If `hash signing: DISABLED`, enable it (see Setup)
or the real signer will be rejected.

## Use

`-ledger-sign-cmd` is just a shell command. The app's default already points at
the built file (relative to the `go/` root):

```bash
go run ./apps/sign-customjson -account myaccount -id vsc.call -json '{...}'
```

which is equivalent to:

```bash
go run ./apps/sign-customjson ... \
  -ledger-sign-cmd "node apps/sign-customjson/ledger-signer/dist/sign.js"
```

If you run from elsewhere, pass an absolute path, or run the package's `sign`
script scoped to this directory:

```bash
go run ./apps/sign-customjson ... \
  -ledger-sign-cmd "pnpm -s -C /path/to/apps/sign-customjson/ledger-signer run sign"
```

If the command isn't installed/runnable, the app exits with an error and points
back here.

### No-build alternative (Node ≥ 22.6)

Recent Node can run the TypeScript directly via type stripping, skipping the
build step entirely:

```bash
go run ./apps/sign-customjson ... \
  -ledger-sign-cmd "node --experimental-strip-types apps/sign-customjson/ledger-signer/sign.ts"
```

(You still need `pnpm install` for the runtime dependencies; you just don't need
`dist/`. The experimental notice Node prints goes to stderr and is harmless.)

## Notes

`-ledger-path` defaults to `m/48'/13'/1'/0'/0'`. Find the right path/account
with the bundled `findpath` helper (see above) or another Hive-Ledger tool if
you're unsure which index holds your authority.

Testnets use a non-default Hive chain id. Because this signer signs a
precomputed digest, the chain id is already folded into `signing_digest` by the
Go side (`sha256(chain_id || serialized_tx)`) — there is nothing chain-specific
to configure here.
