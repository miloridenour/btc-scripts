package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"vsc-node/lib/dids"

	blsu "github.com/protolambda/bls12-381-util"
)

func main() {
	fmt.Fprint(os.Stderr, "paste BlsPrivKeySeed (hex): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		must(err)
	}
	seedBytes, err := hex.DecodeString(strings.TrimSpace(line))
	must(err)
	if len(seedBytes) != 32 {
		fmt.Fprintf(os.Stderr, "seed not 32 bytes (got %d)\n", len(seedBytes))
		os.Exit(2)
	}
	var seed [32]byte
	copy(seed[:], seedBytes)
	var sk blsu.SecretKey
	must(sk.Deserialize(&seed))
	pk, err := blsu.SkToPk(&sk)
	must(err)
	did, err := dids.NewBlsDID(pk)
	must(err)
	fmt.Println(did)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
