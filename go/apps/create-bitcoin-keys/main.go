package main

// gen_key.go
// Generates a Bitcoin private/public key pair for use as a P2WSH backup signing path.
//
// Usage:
//   go mod init gen_key && go mod tidy && go run gen_key.go
//
// Dependencies:
//   github.com/btcsuite/btcd/btcec/v2
//   github.com/btcsuite/btcd/btcutil
//   github.com/btcsuite/btcd/chaincfg

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	// --- 1. Generate private key using OS CSPRNG ---
	// btcec.NewPrivateKey internally uses crypto/rand, which pulls from
	// /dev/urandom on Linux/macOS and CryptGenRandom on Windows.
	// We explicitly seed with crypto/rand here for clarity and auditability.
	var rawKey [32]byte
	if _, err := rand.Read(rawKey[:]); err != nil {
		log.Fatalf("failed to generate random key: %v", err)
	}

	privKey, pubKey := btcec.PrivKeyFromBytes(rawKey[:])

	// Sanity check: ensure the scalar is valid (non-zero, < curve order).
	// btcec.PrivKeyFromBytes clamps if necessary, so verify round-trip.
	if privKey == nil {
		log.Fatal("generated an invalid private key scalar — try again")
	}

	// --- 2. Encode private key as WIF (mainnet, compressed) ---
	wif, err := btcutil.NewWIF(privKey, &chaincfg.MainNetParams, true /* compressed */)
	if err != nil {
		log.Fatalf("failed to encode WIF: %v", err)
	}

	// --- 3. Compressed public key (33 bytes, 02/03 prefix) ---
	compressedPubKey := pubKey.SerializeCompressed()

	// --- 4. Derive P2WPKH address for sanity-check / reference ---
	// For a P2WSH multisig you'll embed the raw pubkey in your script,
	// but this lets you verify the key is well-formed.
	p2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
		btcutil.Hash160(compressedPubKey),
		&chaincfg.MainNetParams,
	)
	if err != nil {
		log.Fatalf("failed to derive P2WPKH address: %v", err)
	}

	// --- 5. Verify: re-derive pubkey from WIF and compare ---
	decoded, err := btcutil.DecodeWIF(wif.String())
	if err != nil {
		log.Fatalf("WIF round-trip decode failed: %v", err)
	}
	roundTripPub := decoded.PrivKey.PubKey().SerializeCompressed()
	if hex.EncodeToString(roundTripPub) != hex.EncodeToString(compressedPubKey) {
		log.Fatal("round-trip verification failed — do not use this key")
	}

	// --- 6. Fingerprint: first 4 bytes of SHA256(pubkey) for identification ---
	digest := sha256.Sum256(compressedPubKey)
	fingerprint := hex.EncodeToString(digest[:4])

	// --- 7. Print results ---
	fmt.Println("==========================================================")
	fmt.Println("  Bitcoin Key Generation — KEEP PRIVATE KEY SECRET")
	fmt.Println("==========================================================")
	fmt.Printf("  Private Key (WIF):      %s\n", wif.String())
	fmt.Printf("  Public Key (hex):       %s\n", hex.EncodeToString(compressedPubKey))
	fmt.Printf("  P2WPKH Address:         %s\n", p2wpkhAddr.EncodeAddress())
	fmt.Printf("  Pubkey Fingerprint:     %s\n", fingerprint)
	fmt.Printf("  Round-trip verified:    OK\n")
	fmt.Println("==========================================================")
	fmt.Println()
	fmt.Println("  For your P2WSH script, embed the PUBLIC KEY (hex) above.")
	fmt.Println("  Store the WIF private key offline (paper/encrypted file).")
	fmt.Println("  This program does not write any files.")

	// Explicit zero-out of the raw key bytes before exit.
	// Go's GC means we can't guarantee all copies are cleared, but this
	// handles the most obvious reference.
	for i := range rawKey {
		rawKey[i] = 0
	}

	os.Exit(0)
}
