package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/miloridenour/vsc-scripts/packages/createaddress"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
	"github.com/miloridenour/vsc-scripts/packages/mempool"
	"github.com/miloridenour/vsc-scripts/packages/transactions"
)

type spendConfig struct {
	BackupPrivateKey string `json:"backup_private_key"`
	PrimaryPubKey    string `json:"primary_pub_key"`
	BackupPubKey     string `json:"backup_pub_key"`
	Instruction      string `json:"instruction"`
	CsvBlocks        int64  `json:"csv_blocks"`
	TxId             string `json:"txid"`
	Vout             uint32 `json:"vout"`
	SendAddress      string `json:"send_address"`
	SendAmount       int64  `json:"send_amount"`
	FeeAmount        int64  `json:"fee_amount"`
	DeductFee        bool   `json:"deduct_fee"`
}

func main() {
	noBroadcast := flag.Bool("nobroadcast", false, "prevents transaction from being broadcast to the bitcoin network")
	networkFlag := flag.String("network", "testnet4", "bitcoin network (mainnet, testnet, testnet4)")
	flag.Parse()

	var network *chaincfg.Params
	switch *networkFlag {
	case "mainnet":
		network = &chaincfg.MainNetParams
	case "testnet":
		network = &chaincfg.TestNet3Params
	case "testnet4":
		network = &chaincfg.TestNet4Params
	default:
		log.Fatalf("unknown network: %s (use mainnet, testnet, or testnet4)", *networkFlag)
	}

	var cfg spendConfig
	err := inputconfig.LoadConfig(&cfg)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Printf("config file created\n")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	// Fetch UTXO details from mempool.space
	mempoolClient := mempool.NewMempoolClient(&http.Client{}, mempool.BaseURLForNetwork(*networkFlag))
	txInfo, err := mempoolClient.GetTx(cfg.TxId)
	if err != nil {
		log.Fatalf("error fetching tx from mempool: %s", err.Error())
	}

	if int(cfg.Vout) >= len(txInfo.Vout) {
		log.Fatalf("vout index %d out of range (tx has %d outputs)", cfg.Vout, len(txInfo.Vout))
	}

	utxoAmount := txInfo.Vout[cfg.Vout].Value
	log.Printf("UTXO %s:%d — %d sats", cfg.TxId, cfg.Vout, utxoAmount)

	// Build the witness script tag from the instruction
	var tag []byte
	if len(cfg.Instruction) > 0 {
		hasher := sha256.New()
		hasher.Write([]byte(cfg.Instruction))
		tag = hasher.Sum(nil)
	} else {
		tag = nil
	}

	// Create the P2WSH address with primary + backup paths
	myAddress, witnessScript, err := createaddress.CreateP2WSHAddressWithBackup(
		cfg.PrimaryPubKey, cfg.BackupPubKey, tag, cfg.CsvBlocks, network,
	)
	if err != nil {
		log.Fatalf("error creating P2WSH address: %s", err)
	}

	log.Printf("Spending From Address: %s", myAddress)

	// Calculate destination and change amounts based on fee mode
	var destAmount, feeAmount int64
	feeAmount = cfg.FeeAmount

	if cfg.DeductFee {
		// Fee is deducted from the send amount: receiver gets (send - fee)
		destAmount = cfg.SendAmount - feeAmount
		if destAmount <= 0 {
			log.Fatalf("send_amount (%d) must be greater than fee_amount (%d) in deduct_fee mode", cfg.SendAmount, feeAmount)
		}
	} else {
		// Fee is on top: receiver gets the full send amount
		destAmount = cfg.SendAmount
	}

	utxo := &transactions.Utxo{
		TxId:   cfg.TxId,
		Vout:   cfg.Vout,
		Amount: utxoAmount,
	}

	// The change goes back to the same address
	// In deduct mode: change = utxo - sendAmount
	// In on-top mode: change = utxo - sendAmount - fee
	var changeFee int64
	if cfg.DeductFee {
		changeFee = 0 // fee already deducted from destAmount
	} else {
		changeFee = feeAmount
	}

	tx, sigHash, err := transactions.CreateBackupSpendTransaction(
		utxo,
		witnessScript,
		cfg.SendAddress,
		myAddress,
		destAmount,
		changeFee,
		cfg.CsvBlocks,
		network,
	)
	if err != nil {
		log.Fatalf("error creating transaction: %s", err.Error())
	}

	log.Printf("sigHash to sign: %x", sigHash)

	signature, err := transactions.SignTxBytes(cfg.BackupPrivateKey, sigHash)
	if err != nil {
		log.Fatalf("error signing: %s", err.Error())
	}

	txPairOut, err := transactions.AttachBackupSignature(tx, witnessScript, signature)
	if err != nil {
		log.Fatalf("error attaching signature: %s", err.Error())
	}

	outputJson, err := json.MarshalIndent(txPairOut, "", "  ")
	if err != nil {
		log.Fatalf("error marshalling output: %s", err.Error())
	}

	fmt.Println(string(outputJson))

	if !*noBroadcast {
		err = mempoolClient.PostTx(txPairOut.RawTx)
		if err != nil {
			log.Fatalf("error broadcasting: %s", err.Error())
		}
		log.Printf("Transaction broadcast successfully: %s", txPairOut.TxId)
	}
}
