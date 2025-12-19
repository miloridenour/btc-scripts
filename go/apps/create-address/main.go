package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/miloridenour/vsc-scripts/packages/createaddress"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

type pubKeyConfig struct {
	PubKey      string `json:"public_key"`
	Instruction string `json:"deposit_instruction"`
}

type addressData struct {
	Address       string `json:"address"`
	WitnessScript string `json:"witness_script"`
}

func main() {
	var keyConfig pubKeyConfig
	err := inputconfig.LoadConfig(&keyConfig)
	if err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Printf("config file created\n")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	network := &chaincfg.TestNet3Params

	var tag []byte
	if len(keyConfig.Instruction) > 0 {
		hasher := sha256.New()
		hasher.Write([]byte(keyConfig.Instruction))
		tag = hasher.Sum(nil)
	} else {
		tag = nil
	}

	address, witnessScript, err := createaddress.CreateP2WSHAddress(keyConfig.PubKey, tag, network)
	if err != nil {
		log.Fatalf("error creating P2WSH address: %s", err.Error())
	}

	output := addressData{
		Address:       address,
		WitnessScript: hex.EncodeToString(witnessScript),
	}

	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("error marshalling json output: %s", err.Error())
	}

	fmt.Println(string(jsonOutput))
}
