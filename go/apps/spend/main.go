package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/miloridenour/vsc-scripts/packages/createaddress"
	"github.com/miloridenour/vsc-scripts/packages/httptransport"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
	"github.com/miloridenour/vsc-scripts/packages/mempool"
	"github.com/miloridenour/vsc-scripts/packages/transactions"
)

type signerConfig struct {
	PrivateKey  string            `json:"private_key"`
	PubKey      string            `json:"public_key"`
	Instruction string            `json:"deposit_instruction"`
	SendAddress string            `json:"send_address"`
	Utxo        transactions.Utxo `json:"utxo"`
	SendAmount  int64             `json:"send_amount"`
	FeeAmount   int64             `json:"fee_amount"`
}

func main() {
	noBroadcast := flag.Bool("nobroadcast", false, "prevents transaction from being broadcast to the bitcoin network")
	flag.Parse()

	network := &chaincfg.TestNet4Params

	var signerConfig signerConfig
	err := inputconfig.LoadConfig(&signerConfig)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Printf("config file created\n")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	var tag []byte
	if len(signerConfig.Instruction) > 0 {
		hasher := sha256.New()
		hasher.Write([]byte(signerConfig.Instruction))
		tag = hasher.Sum(nil)
	} else {
		tag = nil
	}

	myAddress, witnessScript, err := createaddress.CreateP2WSHAddress(signerConfig.PubKey, tag, network)
	if err != nil {
		log.Fatalf("error creating P2WSH address: %s", err)
	}

	log.Printf("Sending From Address: %s\n", myAddress)

	tx, sigHash, err := transactions.CreateSpendTransaction(
		&signerConfig.Utxo,
		witnessScript,
		signerConfig.SendAddress,
		myAddress,
		signerConfig.SendAmount, // amount to send
		signerConfig.FeeAmount,
		network,
	)

	if err != nil {
		log.Fatalf("%s", err.Error())
	}

	log.Printf("sigHash to sign: %x", sigHash)

	signature, err := transactions.SignTxBytes(signerConfig.PrivateKey, sigHash)
	if err != nil {
		log.Fatalf("error signing: %s", err.Error())
	}

	txPairOut, err := transactions.AttachSignature(tx, witnessScript, signature)
	if err != nil {
		log.Fatalf("error attaching signature: %s", err.Error())
	}

	outputJson, err := json.MarshalIndent(txPairOut, "", "  ")
	if err != nil {
		log.Fatalf("error marshalling output: %s", err.Error())
	}

	fmt.Println(string(outputJson))
	// braodcast at: https://blockstream.info/testnet/tx/push

	if !*noBroadcast {
		loggingClient := httptransport.NewLoggingClient()
		mempoolClient := mempool.NewMempoolClient(loggingClient)
		mempoolClient.PostTx(txPairOut.RawTx)
	}
}
