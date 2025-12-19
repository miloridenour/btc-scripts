package main

import (
	"fmt"
	"log"

	"github.com/miloridenour/vsc-scripts/packages/transactions"
)

// hex
const PRIVATEKEY = "a9f4639b99f21599e3cc529567848119c7c6939e00bdb90b7c9c2d5974f3abea"
const sigHashHex = "70e6df1a91ebe2c4b425c28c09e47433a0620d8b60f641fda57c481a956fe521"

func main() {
	signature, err := transactions.SignTx(PRIVATEKEY, sigHashHex)
	if err != nil {
		log.Fatalf("error signing tx: %s", err.Error())
	}
	fmt.Printf("signature: %s\n", signature)
}
