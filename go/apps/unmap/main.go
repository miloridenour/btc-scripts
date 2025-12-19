package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

type unmapConfig struct {
	Amount  int64  `json:"amount"`
	Address string `json:"recipient_btc_address"`
}

func main() {
	var cfg unmapConfig
	err := inputconfig.LoadConfig(&cfg)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Printf("config file created\n")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	txJson, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("error marshalling input: %s", err.Error())
	}

	err = callcontract.CallContract(txJson, "unmap", 10000)
	if err != nil {
		log.Fatalf("error calling contract: %s", err.Error())
	}
}
