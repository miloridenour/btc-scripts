package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
	"github.com/miloridenour/vsc-scripts/packages/mempool"
)

// Config mirrors the sign-load-test scriptInputs.json shape: Hive credentials
// only, with the contract supplied via -contractID at the command line.
type Config struct {
	HiveActiveKey string `json:"HiveActiveKey"`
	HiveUsername  string `json:"HiveUsername"`
	HiveURI       string `json:"HiveURI"`
	HiveChainID   string `json:"HiveChainID"`
	VscNetID      string `json:"VscNetID"`
}

func main() {
	start := flag.Int64("start", -1, "start block height (inclusive)")
	end := flag.Int64("end", -1, "end block height (inclusive)")
	network := flag.String(
		"network",
		"testnet",
		"bitcoin network to fetch headers from: mainnet | testnet | testnet4",
	)
	contractID := flag.String("contractID", "", "btc-mapping-contract ID to call")
	action := flag.String("action", "replaceBlocks", "contract action to call")
	rcLimit := flag.Uint("rc-limit", 20000, "RC limit for the contract call")
	init := flag.Bool("init", false, "create a blank scriptInputs.json and exit")
	flag.Parse()

	var cfg Config
	err := inputconfig.LoadConfig(&cfg)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound || *init {
			fmt.Println("config file created")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}
	if *init {
		fmt.Println("config file created")
		return
	}

	if *start < 0 || *end < 0 {
		log.Fatal("-start and -end are required and must be >= 0")
	}
	if *end < *start {
		log.Fatalf("-end (%d) must be >= -start (%d)", *end, *start)
	}
	if len(*contractID) != 38 || !strings.HasPrefix(*contractID, "vsc1") {
		log.Fatal("contract ID invalid")
	}

	hiveConfig := callcontract.HiveConfig{
		ActiveKey:  cfg.HiveActiveKey,
		Username:   cfg.HiveUsername,
		URI:        cfg.HiveURI,
		ChainID:    cfg.HiveChainID,
		VscNetID:   cfg.VscNetID,
		ContractID: *contractID,
	}

	client := mempool.NewMempoolClient(&http.Client{}, mempool.BaseURLForNetwork(*network))
	fmt.Printf("fetching headers %d..%d from %s\n", *start, *end, mempool.BaseURLForNetwork(*network))

	// Concatenate the hex-encoded 80-byte headers in ascending height order with
	// no separator; replaceBlocks derives the block count N from the payload
	// length and replaces (end-(N-1))..end with these headers.
	var headers strings.Builder
	for h := *start; h <= *end; h++ {
		hash, err := client.GetBlockHashAtHeight(uint32(h))
		if err != nil {
			log.Fatalf("error fetching block hash at height %d: %s", h, err.Error())
		}
		header, err := client.GetBlockHeader(hash)
		if err != nil {
			log.Fatalf("error fetching block header at height %d: %s", h, err.Error())
		}
		if len(header) != 160 {
			log.Fatalf("unexpected header length %d at height %d (want 160 hex chars)", len(header), h)
		}
		headers.WriteString(header)
		fmt.Printf("  [%d] %s\n", h, hash)
	}

	n := *end - *start + 1
	payload, err := json.Marshal(headers.String())
	if err != nil {
		log.Fatalf("error marshalling payload: %s", err.Error())
	}

	fmt.Printf("calling %s with %d block headers (%d hex chars)\n", *action, n, headers.Len())
	if err := callcontract.CallContract(hiveConfig, payload, *action, *rcLimit); err != nil {
		log.Fatalf("contract call failed: %s", err.Error())
	}
}
