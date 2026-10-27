package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vsc-eco/hivego"
)

// runOperations signs and broadcasts an arbitrary set of Hive operations.
//
// Unlike the custom_json mode, nothing here knows or cares what the operations
// are: the user supplies them whole, and the transaction around them (TaPoS
// reference block and expiration) is filled in from the chain head. The
// operations are then serialized by the Hive nodes themselves, which is the
// only way to get the wire bytes for an operation type this tool has no
// serializer for. A malformed or incomplete operation is rejected by the node,
// not by this tool.
func runOperations(a args) error {
	ops, err := readOperations(a.opsPayload, a.opsFile)
	if err != nil {
		return err
	}
	api, err := apiForOperations(ops)
	if err != nil {
		return err
	}

	endpoints := splitCsv(a.api)
	hiveClient := hivego.NewHiveRpc(endpoints)
	hiveClient.ChainID = a.chainId

	t, err := fetchTapos(hiveClient, a.expiration)
	if err != nil {
		return fmt.Errorf("failed to build transaction for signing: %w", err)
	}
	tx := wireTx{
		RefBlockNum:    t.refBlockNum,
		RefBlockPrefix: t.refBlockPrefix,
		Expiration:     t.expiration,
		Operations:     ops,
		Extensions:     []string{},
		Signatures:     []string{},
	}
	txJs, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode transaction: %w", err)
	}

	serialized, err := serializeTxViaNode(endpoints, api, txJs)
	if err != nil {
		return err
	}
	digest := hivego.HashTxForSig(serialized, a.chainId)
	txid := hex.EncodeToString(hashTx(serialized))[:40]

	bundle := signingBundle{
		ChainId:       a.chainId,
		Account:       a.account,
		SigningDigest: hex.EncodeToString(digest),
		SerializedTx:  hex.EncodeToString(serialized),
		TxId:          txid,
		Transaction:   txJs,
		Path:          a.ledgerPath,
	}
	req, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to encode signing request: %w", err)
	}

	fmt.Println("transaction to sign:")
	fmt.Println(string(txJs))
	if a.account != "" {
		fmt.Printf("requesting signature for account %q (path %s)\n", a.account, a.ledgerPath)
	} else {
		fmt.Printf("requesting signature with path %s\n", a.ledgerPath)
	}
	fmt.Println("confirm the hash on your device — the transaction expires at", t.expiration, "UTC")
	fmt.Println("txid will be", txid)

	sig, err := runExternalSigner(a.ledgerSignCmd, req)
	if err != nil {
		return fmt.Errorf(
			"external signer failed: %w\ncommand: %s\n(is the signer set up? see apps/sign-customjson/ledger-signer/README.md)",
			err,
			a.ledgerSignCmd,
		)
	}

	signedJs, err := withSignature(txJs, sig)
	if err != nil {
		return fmt.Errorf("failed to attach signature: %w", err)
	}

	if err := emitSigned(a, signedJs, txid); err != nil {
		return err
	}
	if a.noBroadcast {
		return nil
	}

	if err := broadcastTxViaNode(endpoints, api, signedJs); err != nil {
		return err
	}
	fmt.Println("tx id:", txid)
	return nil
}

// readOperations pulls the operations out of the user's JSON, accepting either
// a bare array of operations or the {"operations": [...]} wrapper that Hive
// tooling prints. Anything alongside "operations" is ignored: the transaction
// itself is built here, so a stale ref block or expiration pasted in with the
// operations would be replaced anyway.
func readOperations(inline string, file string) ([]json.RawMessage, error) {
	raw, err := readJSONInput(inline, file)
	if err != nil {
		return nil, fmt.Errorf("failed to read operations JSON: %w", err)
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("operations JSON is empty")
	}
	if trimmed[0] == '{' {
		var wrapper struct {
			Operations json.RawMessage `json:"operations"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err != nil {
			return nil, fmt.Errorf("operations JSON is not valid: %w", err)
		}
		if len(wrapper.Operations) == 0 {
			return nil, fmt.Errorf(
				"JSON object has no \"operations\" — pass {\"operations\": [...]} or a bare array of operations",
			)
		}
		trimmed = wrapper.Operations
	}

	var ops []json.RawMessage
	if err := json.Unmarshal(trimmed, &ops); err != nil {
		return nil, fmt.Errorf("\"operations\" is not a JSON array: %w", err)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("no operations to sign")
	}
	return ops, nil
}

// hiveApi is the pair of RPC methods that speak one of Hive's two operation
// encodings. The node decides how to read an operation from the API it was
// asked on, so the encoding the user wrote determines which API to use.
type hiveApi struct {
	// encoding names the operation form, for error messages.
	encoding string
	// serializeMethod returns the transaction's wire bytes.
	serializeMethod string
	// broadcastMethod submits the signed transaction.
	broadcastMethod string
	// wrapTrx builds the params for both methods.
	wrapTrx func(tx json.RawMessage) any
	// parseHex reads the serialized hex out of serializeMethod's result.
	parseHex func(result json.RawMessage) (string, error)
}

// apiForOperations picks the API that matches how the operations are written:
//
//	{"type": "transfer_operation", "value": {...}}  -> database_api / network_broadcast_api
//	["transfer", {...}]                             -> condenser_api
//
// The two are not interchangeable — each API rejects the other's form outright,
// and they also differ on assets (NAI objects vs "1.000 HIVE" strings).
func apiForOperations(ops []json.RawMessage) (hiveApi, error) {
	typed := hiveApi{
		encoding:        "typed ({\"type\": ..., \"value\": ...})",
		serializeMethod: "database_api.get_transaction_hex",
		broadcastMethod: "network_broadcast_api.broadcast_transaction",
		wrapTrx:         func(tx json.RawMessage) any { return map[string]any{"trx": tx} },
		parseHex: func(result json.RawMessage) (string, error) {
			var r struct {
				Hex string `json:"hex"`
			}
			if err := json.Unmarshal(result, &r); err != nil {
				return "", fmt.Errorf("unexpected result %s", result)
			}
			return r.Hex, nil
		},
	}
	legacy := hiveApi{
		encoding:        "legacy ([\"name\", {...}])",
		serializeMethod: "condenser_api.get_transaction_hex",
		broadcastMethod: "condenser_api.broadcast_transaction",
		wrapTrx:         func(tx json.RawMessage) any { return []any{tx} },
		parseHex: func(result json.RawMessage) (string, error) {
			var txHex string
			if err := json.Unmarshal(result, &txHex); err != nil {
				return "", fmt.Errorf("unexpected result %s", result)
			}
			return txHex, nil
		},
	}

	var chosen *hiveApi
	for i, op := range ops {
		var candidate *hiveApi
		switch bytes.TrimSpace(op)[0] {
		case '{':
			candidate = &typed
		case '[':
			candidate = &legacy
		default:
			return hiveApi{}, fmt.Errorf(
				"operation %d is neither {\"type\": ..., \"value\": ...} nor [\"name\", {...}]: %s",
				i, op,
			)
		}
		if chosen == nil {
			chosen = candidate
			continue
		}
		if chosen != candidate {
			return hiveApi{}, fmt.Errorf(
				"operations mix encodings — operation %d is %s but an earlier one is %s; "+
					"Hive reads a transaction as one form or the other, so all of them must match",
				i, candidate.encoding, chosen.encoding,
			)
		}
	}
	return *chosen, nil
}

// serializeTxViaNode asks the Hive nodes for the transaction's wire bytes.
//
// The node is doing the serialization, which means a lying endpoint could hand
// back the bytes of a *different* transaction — and because the Ledger signs the
// digest blind (it displays only a hash), that substitution would not be visible
// on the device. Every configured endpoint is therefore asked and all of them
// must agree, so passing two or three independent -api endpoints turns a single
// compromised node into a hard failure instead of a signed transaction.
func serializeTxViaNode(endpoints []string, api hiveApi, txJs []byte) ([]byte, error) {
	var (
		agreed   string
		agreedBy string
		failures []string
	)
	for _, ep := range endpoints {
		res, err := rpcCall(ep, api.serializeMethod, api.wrapTrx(json.RawMessage(txJs)))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ep, err))
			continue
		}
		txHex, err := api.parseHex(res)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ep, err))
			continue
		}
		if agreed == "" {
			agreed, agreedBy = txHex, ep
			continue
		}
		if txHex != agreed {
			return nil, fmt.Errorf(
				"endpoints disagree on the serialized transaction — refusing to sign\n  %s: %s\n  %s: %s",
				agreedBy, agreed, ep, txHex,
			)
		}
	}
	if agreed == "" {
		return nil, fmt.Errorf(
			"no endpoint could serialize the transaction:\n  %s",
			strings.Join(failures, "\n  "),
		)
	}
	if len(failures) > 0 {
		fmt.Println("warning: some endpoints could not serialize the transaction:")
		for _, f := range failures {
			fmt.Println("  " + f)
		}
	}

	b, err := hex.DecodeString(agreed)
	if err != nil {
		return nil, fmt.Errorf("node returned an invalid serialized transaction %q: %w", agreed, err)
	}
	// get_transaction_hex serializes the whole transaction including the signature
	// array; the digest covers everything before it. Drop the (empty) count byte.
	if len(b) == 0 || b[len(b)-1] != 0x00 {
		return nil, fmt.Errorf("node returned a serialized transaction that is not unsigned: %s", agreed)
	}
	return b[:len(b)-1], nil
}

// broadcastTxViaNode broadcasts the signed transaction, stopping at the first
// endpoint that accepts it.
func broadcastTxViaNode(endpoints []string, api hiveApi, signedTx []byte) error {
	var failures []string
	for _, ep := range endpoints {
		if _, err := rpcCall(ep, api.broadcastMethod, api.wrapTrx(json.RawMessage(signedTx))); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ep, err))
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to broadcast signed transaction:\n  %s", strings.Join(failures, "\n  "))
}

// withSignature appends sig to the transaction's signatures array, keeping any
// signatures already present so a multi-signature transaction can be passed
// from signer to signer.
func withSignature(txJs []byte, sig string) ([]byte, error) {
	var tx wireTx
	if err := json.Unmarshal(txJs, &tx); err != nil {
		return nil, err
	}
	tx.Signatures = append(tx.Signatures, sig)
	return json.MarshalIndent(tx, "", "  ")
}

func hashTx(serialized []byte) []byte {
	sum := sha256.Sum256(serialized)
	return sum[:]
}

type rpcRequest struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	Id      int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// rpcCall posts a single JSON-RPC request. hivego has no get_transaction_hex
// binding and its broadcast path wants typed operations, so this mode talks to
// the node directly.
func rpcCall(endpoint string, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{Jsonrpc: "2.0", Method: method, Params: params, Id: 1})
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("unreadable response: %s", strings.TrimSpace(string(raw)))
	}
	if len(parsed.Error) > 0 && string(parsed.Error) != "null" {
		return nil, fmt.Errorf("rpc error: %s", rpcErrorMessage(parsed.Error))
	}
	return parsed.Result, nil
}

// rpcErrorMessage pulls the readable message out of a Hive error object. Hive
// attaches a large assertion stack that buries the one line worth reading.
func rpcErrorMessage(raw json.RawMessage) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &e); err == nil && e.Message != "" {
		return e.Message
	}
	return string(raw)
}
