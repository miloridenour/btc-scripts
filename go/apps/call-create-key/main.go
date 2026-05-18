package main

import (
	"fmt"
	"log"
	"os"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

type Config struct {
	HiveActiveKey string `json:"HiveActiveKey"`
	HiveUsername  string `json:"HiveUsername"`
	HiveURI      string `json:"HiveURI"`
}

func main() {
	var cfg Config
	err := inputconfig.LoadConfig(&cfg)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Println("config file created")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	hiveConfig := callcontract.HiveConfig{
		ActiveKey: cfg.HiveActiveKey,
		Username:  cfg.HiveUsername,
		URI:       cfg.HiveURI,
	}

	err = callcontract.CallContract(hiveConfig, []byte(""), "create_key_pair")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
}
