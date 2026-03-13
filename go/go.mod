module github.com/miloridenour/vsc-scripts

go 1.25.1

replace github.com/agl/ed25519 => github.com/binance-chain/edwards25519 v0.0.0-20200305024217-f36fc4b53d43

replace github.com/vsc-eco/hivego => /home/milo/Documents/vsc/hivego

require (
	github.com/btcsuite/btcd v0.25.0
	github.com/btcsuite/btcd/btcec/v2 v2.3.5
	github.com/btcsuite/btcd/btcutil v1.1.6
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0
	github.com/vsc-eco/hivego v0.0.0-20250604205027-fa6c9e2c8be7
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/btcsuite/btclog v1.0.0 // indirect
	github.com/cfoxon/jsonrpc2client v0.0.0-20221203070057-deee6c789601 // indirect
	github.com/decred/base58 v1.0.5 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v2 v2.0.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/tinylib/msgp v1.6.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.57.0 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)
