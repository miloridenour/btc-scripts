// verify_key.go
// Derives and prints the public key + fingerprint from a WIF private key.
// Used to verify a paper backup was transcribed correctly.
//
// Usage:
//   go run -mod=vendor verify_key.go
//
// The program reads the WIF from stdin (not as a CLI arg, to avoid shell
// history logging). Type or paste the key, then press Enter.

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	fmt.Println("==========================================================")
	fmt.Println("  Bitcoin WIF Verification")
	fmt.Println("==========================================================")
	fmt.Println("  Enter WIF private key (input is not echoed in some")
	fmt.Println("  terminals; press Enter when done):")
	fmt.Print("  > ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	wifStr := strings.TrimSpace(scanner.Text())

	if wifStr == "" {
		log.Fatal("no input provided")
	}

	// Decode and validate the WIF
	wif, err := btcutil.DecodeWIF(wifStr)
	if err != nil {
		// Give a clear error without echoing the key back to the terminal
		log.Fatalf("invalid WIF: %v\n(check for transcription errors: 0/O, I/l/1, common mistakes)", err)
	}

	if !wif.IsForNet(&chaincfg.MainNetParams) {
		fmt.Println("  WARNING: this WIF is not for mainnet")
	}

	// Derive compressed public key
	pubKey := wif.PrivKey.PubKey().SerializeCompressed()
	pubKeyHex := hex.EncodeToString(pubKey)

	// Fingerprint: first 4 bytes of SHA256(pubkey)
	digest := sha256.Sum256(pubKey)
	fingerprint := hex.EncodeToString(digest[:4])

	// P2WPKH address for reference
	p2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
		btcutil.Hash160(pubKey),
		&chaincfg.MainNetParams,
	)
	if err != nil {
		log.Fatalf("failed to derive address: %v", err)
	}

	fmt.Println()
	fmt.Println("  Derived values — compare against your written record:")
	fmt.Println("----------------------------------------------------------")
	fmt.Printf("  Public Key (hex):   %s\n", pubKeyHex)
	fmt.Printf("  Fingerprint:        %s\n", fingerprint)
	fmt.Printf("  P2WPKH Address:     %s\n", p2wpkhAddr.EncodeAddress())
	fmt.Println("==========================================================")
	fmt.Println()
	fmt.Println("  If fingerprint and public key match your record: OK")
	fmt.Println("  If they differ: the WIF was transcribed incorrectly.")
}
