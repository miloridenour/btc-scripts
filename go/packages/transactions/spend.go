package transactions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

type Utxo struct {
	TxId   string `json:"tx_id"`
	Vout   uint32 `json:"vout"`
	Amount int64  `json:"amount"`
}

func CreateSpendTransaction(
	utxo *Utxo,
	witnessScript []byte,
	destAddress string,
	changeAddress string,
	sendAmount int64,
	feeAmount int64,
	network *chaincfg.Params,
) (*wire.MsgTx, []byte, error) {
	txHash, err := chainhash.NewHashFromStr(utxo.TxId)
	if err != nil {
		return nil, nil, err
	}

	tx := wire.NewMsgTx(wire.TxVersion)

	outPoint := wire.NewOutPoint(txHash, utxo.Vout)
	txIn := wire.NewTxIn(outPoint, nil, nil)
	tx.AddTxIn(txIn)

	destAddr, err := btcutil.DecodeAddress(destAddress, network)
	if err != nil {
		return nil, nil, err
	}

	// Create output script for destination
	destScript, err := txscript.PayToAddrScript(destAddr)
	if err != nil {
		return nil, nil, err
	}

	txOut := wire.NewTxOut(sendAmount, destScript)
	tx.AddTxOut(txOut)

	// change (return to address )
	changeAmount := utxo.Amount - sendAmount - feeAmount
	if changeAmount > 546 { // dust threshold
		change, err := btcutil.DecodeAddress(changeAddress, network)
		if err != nil {
			return nil, nil, err
		}
		changePkScript, err := txscript.PayToAddrScript(change)
		if err != nil {
			return nil, nil, err
		}
		txOutChange := wire.NewTxOut(changeAmount, changePkScript)
		tx.AddTxOut(txOutChange)
	}

	witnessProgram := sha256.Sum256(witnessScript)
	pkscript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(witnessProgram[:]).
		Script()
	if err != nil {
		return nil, nil, err
	}

	log.Printf("pk script:      %s", hex.EncodeToString(pkscript))
	log.Printf("witness script: %s", hex.EncodeToString(witnessScript))

	// Calculate witness sighash (the data to be signed)
	sigHashes := txscript.NewTxSigHashes(tx, txscript.NewCannedPrevOutputFetcher(
		pkscript, utxo.Amount))

	witnessHash, err := txscript.CalcWitnessSigHash(
		witnessScript,
		sigHashes,
		txscript.SigHashAll,
		tx,
		0, // input index
		utxo.Amount,
	)
	if err != nil {
		return nil, nil, err
	}

	log.Printf("witness hash:   %s", hex.EncodeToString(witnessHash))

	return tx, witnessHash, nil
}

// CreateBackupSpendTransaction creates a transaction spending via the backup (CSV) path.
// It sets tx version 2 and the input sequence to csvBlocks for OP_CHECKSEQUENCEVERIFY.
func CreateBackupSpendTransaction(
	utxo *Utxo,
	witnessScript []byte,
	destAddress string,
	changeAddress string,
	sendAmount int64,
	feeAmount int64,
	csvBlocks int64,
	network *chaincfg.Params,
) (*wire.MsgTx, []byte, error) {
	txHash, err := chainhash.NewHashFromStr(utxo.TxId)
	if err != nil {
		return nil, nil, err
	}

	// Version 2 required for CSV
	tx := wire.NewMsgTx(2)

	outPoint := wire.NewOutPoint(txHash, utxo.Vout)
	txIn := wire.NewTxIn(outPoint, nil, nil)
	txIn.Sequence = uint32(csvBlocks) // must satisfy OP_CHECKSEQUENCEVERIFY
	tx.AddTxIn(txIn)

	destAddr, err := btcutil.DecodeAddress(destAddress, network)
	if err != nil {
		return nil, nil, err
	}

	destScript, err := txscript.PayToAddrScript(destAddr)
	if err != nil {
		return nil, nil, err
	}

	txOut := wire.NewTxOut(sendAmount, destScript)
	tx.AddTxOut(txOut)

	// change output
	changeAmount := utxo.Amount - sendAmount - feeAmount
	if changeAmount > 546 { // dust threshold
		change, err := btcutil.DecodeAddress(changeAddress, network)
		if err != nil {
			return nil, nil, err
		}
		changePkScript, err := txscript.PayToAddrScript(change)
		if err != nil {
			return nil, nil, err
		}
		txOutChange := wire.NewTxOut(changeAmount, changePkScript)
		tx.AddTxOut(txOutChange)
	}

	witnessProgram := sha256.Sum256(witnessScript)
	pkscript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(witnessProgram[:]).
		Script()
	if err != nil {
		return nil, nil, err
	}

	log.Printf("pk script:      %s", hex.EncodeToString(pkscript))
	log.Printf("witness script: %s", hex.EncodeToString(witnessScript))

	sigHashes := txscript.NewTxSigHashes(tx, txscript.NewCannedPrevOutputFetcher(
		pkscript, utxo.Amount))

	witnessHash, err := txscript.CalcWitnessSigHash(
		witnessScript,
		sigHashes,
		txscript.SigHashAll,
		tx,
		0,
		utxo.Amount,
	)
	if err != nil {
		return nil, nil, err
	}

	log.Printf("witness hash:   %s", hex.EncodeToString(witnessHash))

	return tx, witnessHash, nil
}

// AttachBackupSignature attaches a signature for the backup (ELSE) path of an IF/ELSE witness script.
// The witness stack is: <signature> <OP_FALSE> <witnessScript>
// OP_FALSE (empty byte slice) causes the IF to take the ELSE branch.
func AttachBackupSignature(tx *wire.MsgTx, witnessScript []byte, signature []byte) (*TxRawIdPair, error) {
	witness := wire.TxWitness{
		signature,
		{}, // OP_FALSE — takes the ELSE branch
		witnessScript,
	}

	tx.TxIn[0].Witness = witness

	var buf bytes.Buffer
	if err := tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding); err != nil {
		return nil, err
	}

	return &TxRawIdPair{
		RawTx: hex.EncodeToString(buf.Bytes()),
		TxId:  tx.TxID(),
	}, nil
}
