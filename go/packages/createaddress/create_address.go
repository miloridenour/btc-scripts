package createaddress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"fmt"
	"reflect"
	"runtime"
)

func GetRawTxHex(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func CreateScriptP2SH(pubKeyHex string, tag []byte, network *chaincfg.Params) (string, []byte, error) {
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return "", nil, err
	}

	scriptBuilder := txscript.NewScriptBuilder()
	scriptBuilder.AddData(pubKeyBytes)              // Push pubkey
	scriptBuilder.AddOp(txscript.OP_CHECKSIGVERIFY) // OP_CHECKSIGVERIFY
	scriptBuilder.AddData(tag)                      // Push tag/bits

	script, err := scriptBuilder.Script()
	if err != nil {
		return "", nil, err
	}

	scriptHash := btcutil.Hash160(script)
	address, err := btcutil.NewAddressScriptHash(scriptHash, network)

	if err != nil {
		return "", nil, err
	}

	return address.EncodeAddress(), script, nil
}

func CreateP2WSHAddress(pubKeyHex string, tag []byte, network *chaincfg.Params) (string, []byte, error) {
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return "", nil, err
	}

	scriptBuilder := txscript.NewScriptBuilder()
	if len(tag) > 0 {
		scriptBuilder.AddData(pubKeyBytes)              // Push pubkey
		scriptBuilder.AddOp(txscript.OP_CHECKSIGVERIFY) // OP_CHECKSIGVERIFY
		scriptBuilder.AddData(tag)                      // Push tag/bits
	} else {
		scriptBuilder.AddData(pubKeyBytes)
		scriptBuilder.AddOp(txscript.OP_CHECKSIG)
	}

	script, err := scriptBuilder.Script()
	if err != nil {
		return "", nil, err
	}

	var address string
	witnessProgram := sha256.Sum256(script)
	addressWitnessScriptHash, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], network)
	if err != nil {
		return "", nil, err
	}
	address = addressWitnessScriptHash.EncodeAddress()

	return address, script, nil
}

func CreateMultipleExamples() {
	pubKey := "0242f9da15eae56fe6aca65136738905c0afdb2c4edf379e107b3b00b98c7fc9f0"
	tag := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	tagBytes, err := hex.DecodeString(tag)
	if err != nil {
		return
	}

	functions := [](func(pubKeyHex string, tag []byte, network *chaincfg.Params) (string, []byte, error)){
		CreateScriptP2SH,
		CreateP2WSHAddress,
	}

	for _, fn := range functions {
		funcName := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
		fmt.Printf("Function: %s\n", funcName)
		// For testnet
		address, _, err := fn(pubKey, tagBytes, &chaincfg.TestNet4Params)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Testnet Address: %s\n", address)

		// For mainnet
		addressMain, _, err := fn(pubKey, tagBytes, &chaincfg.MainNetParams)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Mainnet Address: %s\n", addressMain)
	}

}
