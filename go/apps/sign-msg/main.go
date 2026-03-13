package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/miloridenour/vsc-scripts/packages/httptransport"
	"github.com/miloridenour/vsc-scripts/packages/mempool"
	"github.com/miloridenour/vsc-scripts/packages/transactions"
)

type SignedInput struct {
	Index     uint32 `json:"index"`
	Signature string `json:"signature"`
}

type Output struct {
	TxId       string        `json:"tx_id"`
	RawTx      string        `json:"raw_tx,omitempty"`
	Signatures []SignedInput  `json:"signatures"`
}

func isHex(s string) bool {
	if len(s) == 0 || len(s)%2 != 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func main() {
	keyFlag := flag.String("key", "", "private key hex (required)")
	broadcast := flag.Bool("broadcast", false, "attach signatures to the transaction and broadcast")
	network := flag.String("network", "testnet4", "bitcoin network (mainnet, testnet, testnet4)")
	inputFile := flag.String("file", "", "read msgpack from file instead of stdin")
	outputFile := flag.String("o", "", "write JSON output to file instead of stdout")
	flag.Parse()

	if *keyFlag == "" {
		log.Fatal("private key is required (-key)")
	}

	// Select network params
	var netParams *chaincfg.Params
	switch *network {
	case "mainnet":
		netParams = &chaincfg.MainNetParams
	case "testnet":
		netParams = &chaincfg.TestNet3Params
	case "testnet4":
		netParams = &chaincfg.TestNet4Params
	default:
		log.Fatalf("unknown network: %s", *network)
	}

	// Read msgpack input
	var reader io.Reader
	if *inputFile != "" {
		f, err := os.Open(*inputFile)
		if err != nil {
			log.Fatalf("error opening file: %s", err)
		}
		defer f.Close()
		reader = f
	} else {
		reader = os.Stdin
	}

	raw, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("error reading input: %s", err)
	}

	// Support hex-encoded input (e.g. files produced by the Go tooling)
	trimmed := strings.TrimSpace(string(raw))
	if isHex(trimmed) {
		raw, err = hex.DecodeString(trimmed)
		if err != nil {
			log.Fatalf("error hex-decoding input: %s", err)
		}
	}

	var signingData SigningData
	_, err = signingData.UnmarshalMsg(raw)
	if err != nil {
		log.Fatalf("error decoding msgpack: %s", err)
	}

	// Decode the transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	err = tx.BtcDecode(bytes.NewReader(signingData.Tx), wire.ProtocolVersion, wire.BaseEncoding)
	if err != nil {
		log.Fatalf("error decoding transaction: %s", err)
	}

	// Sign each sighash
	signatures := make([]SignedInput, len(signingData.UnsignedSigHashes))
	for i, ush := range signingData.UnsignedSigHashes {
		sig, err := transactions.SignTxBytes(*keyFlag, ush.SigHash)
		if err != nil {
			log.Fatalf("error signing input %d: %s", ush.Index, err)
		}

		signatures[i] = SignedInput{
			Index:     ush.Index,
			Signature: hex.EncodeToString(sig),
		}

		if *broadcast {
			tx.TxIn[ush.Index].Witness = wire.TxWitness{
				sig,
				ush.WitnessScript,
			}
		}
	}

	output := Output{
		TxId:       tx.TxID(),
		Signatures: signatures,
	}

	if *broadcast {
		var buf bytes.Buffer
		err = tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding)
		if err != nil {
			log.Fatalf("error encoding signed transaction: %s", err)
		}
		output.RawTx = hex.EncodeToString(buf.Bytes())

		_ = netParams // network used for broadcast endpoint selection
		loggingClient := httptransport.NewLoggingClient()
		mempoolClient := mempool.NewMempoolClient(loggingClient)
		err = mempoolClient.PostTx(output.RawTx)
		if err != nil {
			log.Fatalf("error broadcasting transaction: %s", err)
		}
		log.Printf("transaction broadcast successfully")
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("error marshalling output: %s", err)
	}

	if *outputFile != "" {
		err = os.WriteFile(*outputFile, append(out, '\n'), 0644)
		if err != nil {
			log.Fatalf("error writing output file: %s", err)
		}
	} else {
		fmt.Println(string(out))
	}
}
