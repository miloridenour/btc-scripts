package transactions

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func SignTx(privateKeyHex string, sigHashHex string) (string, error) {
	sigHash, err := hex.DecodeString(sigHashHex)
	if err != nil {
		return "", fmt.Errorf("error decoding signature hex: %w", err)
	}

	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("error decoding private key hex: %w", err)
	}
	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)

	signature := ecdsa.Sign(privateKey, sigHash)
	sigBytes := append(signature.Serialize(), byte(txscript.SigHashAll))

	signatureHex := hex.EncodeToString(sigBytes)

	return signatureHex, nil
}

func SignTxBytes(privateKeyHex string, sigHash []byte) ([]byte, error) {
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("error decoding private key hex: %w", err)
	}
	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)

	signature := ecdsa.Sign(privateKey, sigHash)
	log.Printf("signature R: %s", signature.R().String())
	log.Printf("signature S: %s", signature.S().String())

	sigBytes := append(signature.Serialize(), byte(txscript.SigHashAll))

	log.Printf("signature hex: %s", hex.EncodeToString(sigBytes))

	return sigBytes, nil
}

type TxRawIdPair struct {
	RawTx string `json:"raw_tx"`
	TxId  string `json:"tx_id"`
}

// attaches signature for a single utxo transaction (always attaches at index 0)
func AttachSignature(tx *wire.MsgTx, witnessScript []byte, signature []byte) (*TxRawIdPair, error) {
	witness := wire.TxWitness{
		signature,
		witnessScript,
	}

	tx.TxIn[0].Witness = witness

	var buf bytes.Buffer
	// serialize is almost the same but with a different protocol version. Not sure if that
	// actually changes the result
	if err := tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding); err != nil {
		return nil, err
	}

	return &TxRawIdPair{
		RawTx: hex.EncodeToString(buf.Bytes()),
		TxId:  tx.TxID(),
	}, nil
}
