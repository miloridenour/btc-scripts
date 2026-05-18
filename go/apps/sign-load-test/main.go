package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

const (
	// contractID     = "vsc1BaRNeKwCboYacYdeFwh18jQCP85QivkhKT"
	// action         = "signKey"
	hexBytes       = 32
	formatPrefixed = "prefixed"
	formatRaw      = "raw"
	formatJSON     = "json"
)

type Params struct {
	KeyName string `json:"key_name"`
	MsgHex  string `json:"msg_hex,omitempty"`
}

type Config struct {
	HiveActiveKey string `json:"HiveActiveKey"`
	HiveUsername  string `json:"HiveUsername"`
	HiveURI       string `json:"HiveURI"`
	HiveChainID   string `json:"HiveChainID"`
	VscNetID      string `json:"VscNetID"`
}

func main() {
	n := flag.Int("n", 0, "number of sequential calls to make (required)")
	start := flag.Int64("start", -1, "starting counter value; -1 to seed with random 32 bytes")
	rcLimit := flag.Uint("rc-limit", 5000, "RC limit per call")
	format := flag.String(
		"format",
		formatPrefixed,
		"payload format: 'prefixed' (key-id,<hex>), 'raw' (<hex>), or 'json' ({key_name, msg_hex})",
	)
	contractID := flag.String("contractID", "", "Contract ID to call")
	action := flag.String("action", "signKey", "Action to call.")
	keyName := flag.String("keyName", "key", "Name of the key.")
	init := flag.Bool("init", false, "Whether to run init.")
	flag.Parse()

	if !*init && *n <= 0 {
		log.Fatal("-n must be > 0")
	}

	if *format != formatPrefixed && *format != formatRaw && *format != formatJSON {
		log.Fatalf("-format must be %q, %q, or %q", formatPrefixed, formatRaw, formatJSON)
	}

	var cfg Config
	err := inputconfig.LoadConfig(&cfg)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound || *init {
			fmt.Println("config file created")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	if len(*contractID) != 38 || !strings.HasPrefix(*contractID, "vsc1") {
		log.Fatalf("contract ID invalid")
	}

	hiveConfig := callcontract.HiveConfig{
		ActiveKey:  cfg.HiveActiveKey,
		Username:   cfg.HiveUsername,
		URI:        cfg.HiveURI,
		ChainID:    cfg.HiveChainID,
		VscNetID:   cfg.VscNetID,
		ContractID: *contractID,
	}

	counter, err := initCounter(*start)
	if err != nil {
		log.Fatalf("error initializing counter: %s", err.Error())
	}
	one := big.NewInt(1)

	for i := 0; i < *n; i++ {
		hexStr := counterToHex(counter, hexBytes)

		var marshalTarget any
		switch *format {
		case formatPrefixed:
			marshalTarget = *keyName + "," + hexStr
		case formatRaw:
			marshalTarget = hexStr
		case formatJSON:
			marshalTarget = Params{KeyName: *keyName, MsgHex: hexStr}
		}

		payload, err := json.Marshal(marshalTarget)
		if err != nil {
			log.Fatalf("error marshalling payload: %s", err.Error())
		}

		fmt.Printf("[%d/%d] calling %s with payload: %s\n", i+1, *n, *action, string(payload))

		err = callcontract.CallContract(hiveConfig, payload, *action, *rcLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "call %d failed: %s\n", i+1, err.Error())
			os.Exit(1)
		}

		counter.Add(counter, one)
		if i != *n-1 {
			time.Sleep(3 * time.Second)
		}
	}
}

func counterToHex(n *big.Int, numBytes int) string {
	buf := make([]byte, numBytes)
	n.FillBytes(buf)
	return hex.EncodeToString(buf)
}

func initCounter(start int64) (*big.Int, error) {
	if start == -1 {
		buf := make([]byte, hexBytes)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generating random seed: %w", err)
		}
		return new(big.Int).SetBytes(buf), nil
	}
	if start < 0 {
		return nil, fmt.Errorf("-start must be >= 0 or -1, got %d", start)
	}
	return new(big.Int).SetInt64(start), nil
}
