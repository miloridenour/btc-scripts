// Command sign-customjson signs a Hive transaction with a Ledger hardware
// wallet and broadcasts it (or prints the signed transaction with
// -no-broadcast). It has two modes:
//
//   - custom_json (-id/-json): the transaction is built here around a single
//     custom_json operation and serialized with hivego — the same serializer
//     used to broadcast, so the bytes the device signs and the bytes the node
//     broadcasts can never diverge.
//   - operations (-ops/-ops-file): the operations are supplied whole, as JSON,
//     and the transaction around them is built the same way. Any operation type
//     works. See operations.go.
//
// Adapted from go-vsc-node's cmd/contract-deployer Ledger signing harness. In
// both modes the 32-byte signing digest is handed to an external signing
// command (-ledger-sign-cmd, the bundled ledger-signer by default) which asks
// the Ledger Hive app to sign that exact hash.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vsc-eco/hivego"
)

type args struct {
	account       string
	id            string
	jsonPayload   string
	jsonFile      string
	posting       bool
	opsPayload    string
	opsFile       string
	api           string
	chainId       string
	expiration    int
	ledgerSignCmd string
	ledgerPath    string
	noBroadcast   bool
	out           string
}

func parseArgs() (args, error) {
	flag.Usage = func() {
		fmt.Printf("Sign (via Hive Ledger app) and broadcast a Hive transaction.\n\n")
		fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
		fmt.Printf("custom_json mode: -account and -id with -json/-json-file.\n\n")
		fmt.Printf("Operations mode: -ops/-ops-file with the operations to sign, either\n")
		fmt.Printf("{\"operations\": [...]} or a bare array. Any operation type works, in either\n")
		fmt.Printf("Hive encoding ({\"type\": ..., \"value\": ...} or [\"name\", {...}]).\n\n")
		fmt.Printf("Both modes fill in the reference block and expiration from the chain head.\n\n")
		flag.PrintDefaults()
	}
	account := flag.String("account", "", "Hive account authorizing the operation (required for custom_json mode)")
	id := flag.String("id", "", "custom_json id, e.g. \"vsc.call\" (required for custom_json mode)")
	jsonPayload := flag.String("json", "", "Inline JSON payload for the custom_json operation")
	jsonFile := flag.String("json-file", "", "Path to a file containing the custom_json payload (\"-\" for stdin)")
	posting := flag.Bool(
		"posting",
		false,
		"Authorize with the posting authority instead of active. Make sure -ledger-path points at the matching key.",
	)
	opsPayload := flag.String(
		"ops",
		"",
		"Inline operations JSON — {\"operations\": [...]} or a bare array. Selects operations mode.",
	)
	opsFile := flag.String(
		"ops-file",
		"",
		"Path to a file containing the operations JSON (\"-\" for stdin). Selects operations mode.",
	)
	api := flag.String(
		"api",
		"https://api.hive.blog",
		"Comma-separated Hive RPC endpoints. In operations mode every endpoint is asked to serialize the transaction and all must agree, so listing two or three independent ones is worthwhile.",
	)
	chainId := flag.String("chain-id", "", "Hex chain id. Empty selects Hive mainnet.")
	expiration := flag.Int(
		"expiration",
		1800,
		"Seconds until the transaction expires (max 3600). Larger values give more time to confirm on the device.",
	)
	ledgerSignCmd := flag.String(
		"ledger-sign-cmd",
		"node apps/sign-customjson/ledger-signer/dist/sign.js",
		"External command that produces the signature. Run via 'sh -c'; receives the signing request JSON on stdin and must print the signature hex on stdout. The default assumes the bundled ledger-signer is built ('pnpm install' in apps/sign-customjson/ledger-signer/) and that you run from the go/ root.",
	)
	ledgerPath := flag.String(
		"ledger-path",
		"m/48'/13'/1'/0'/0'",
		"BIP32 (SLIP-0048) derivation path passed to the signer. Adjust the role/account components for other keys — the bundled ledger-signer's 'findpath' helper locates the path holding a known public key.",
	)
	noBroadcast := flag.Bool(
		"no-broadcast",
		false,
		"Sign on the device but print the signed transaction JSON instead of broadcasting it.",
	)
	out := flag.String("out", "", "Also write the signed transaction JSON to this file path.")
	flag.Parse()

	parsed := args{
		account:       *account,
		id:            *id,
		jsonPayload:   *jsonPayload,
		jsonFile:      *jsonFile,
		posting:       *posting,
		opsPayload:    *opsPayload,
		opsFile:       *opsFile,
		api:           *api,
		chainId:       *chainId,
		expiration:    *expiration,
		ledgerSignCmd: *ledgerSignCmd,
		ledgerPath:    *ledgerPath,
		noBroadcast:   *noBroadcast,
		out:           *out,
	}

	if parsed.ledgerSignCmd == "" {
		return parsed, fmt.Errorf("-ledger-sign-cmd must not be empty")
	}
	if parsed.api == "" {
		return parsed, fmt.Errorf("-api must not be empty")
	}

	if parsed.expiration <= 0 || parsed.expiration > 3600 {
		return parsed, fmt.Errorf(
			"-expiration must be between 1 and 3600 seconds (Hive's maximum), got %d",
			parsed.expiration,
		)
	}

	if parsed.opsMode() {
		if parsed.opsPayload != "" && parsed.opsFile != "" {
			return parsed, fmt.Errorf("-ops and -ops-file are mutually exclusive")
		}
		// In operations mode the operations are taken as written, so the flags
		// that would build a custom_json for you are meaningless here.
		for flagName, set := range map[string]bool{
			"-id":        parsed.id != "",
			"-json":      parsed.jsonPayload != "",
			"-json-file": parsed.jsonFile != "",
			"-posting":   parsed.posting,
		} {
			if set {
				return parsed, fmt.Errorf(
					"%s builds a custom_json operation and cannot be combined with -ops/-ops-file; "+
						"write the operation out in the operations JSON instead",
					flagName,
				)
			}
		}
		return parsed, nil
	}

	if parsed.account == "" {
		return parsed, fmt.Errorf("-account is required")
	}
	if parsed.id == "" {
		return parsed, fmt.Errorf("-id is required")
	}
	if (parsed.jsonPayload == "") == (parsed.jsonFile == "") {
		return parsed, fmt.Errorf("exactly one of -json or -json-file is required")
	}
	return parsed, nil
}

// opsMode reports whether operations were supplied directly, in which case this
// tool signs those instead of building a custom_json operation.
func (a args) opsMode() bool {
	return a.opsPayload != "" || a.opsFile != ""
}

func main() {
	a, err := parseArgs()
	if err != nil {
		fmt.Println(err)
		flag.Usage()
		os.Exit(1)
	}

	if a.opsMode() {
		if err := runOperations(a); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}

	payload, err := readPayload(a)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	op := hivego.CustomJsonOperation{
		RequiredAuths:        []string{a.account},
		RequiredPostingAuths: []string{},
		Id:                   a.id,
		Json:                 payload,
	}
	if a.posting {
		op.RequiredAuths = []string{}
		op.RequiredPostingAuths = []string{a.account}
	}

	hiveClient := hivego.NewHiveRpc(splitCsv(a.api))
	hiveClient.ChainID = a.chainId

	bundle, tx, err := buildSigningBundle(hiveClient, op, a)
	if err != nil {
		fmt.Println("failed to build transaction for signing:", err)
		os.Exit(1)
	}

	req, err := json.Marshal(bundle)
	if err != nil {
		fmt.Println("failed to encode signing request:", err)
		os.Exit(1)
	}

	fmt.Println("custom_json id:", a.id)
	fmt.Println("payload:", payload)
	fmt.Printf("requesting signature for account %q (path %s)\n", a.account, a.ledgerPath)
	fmt.Println("confirm the hash on your device — the transaction expires at", tx.Expiration, "UTC")

	sig, err := runExternalSigner(a.ledgerSignCmd, req)
	if err != nil {
		fmt.Println("external signer failed:", err)
		fmt.Println("command:", a.ledgerSignCmd)
		fmt.Println("(is the signer set up? see apps/sign-customjson/ledger-signer/README.md)")
		os.Exit(1)
	}

	tx.AddSig(sig)

	signedJs, err := withSignature(bundle.Transaction, sig)
	if err != nil {
		fmt.Println("failed to attach signature:", err)
		os.Exit(1)
	}
	if err := emitSigned(a, signedJs, bundle.TxId); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if a.noBroadcast {
		return
	}

	txid, err := hiveClient.BroadcastRaw(tx)
	if err != nil {
		fmt.Println("failed to broadcast signed transaction:", err)
		os.Exit(1)
	}
	fmt.Println("tx id:", txid)
}

// emitSigned handles -out and -no-broadcast, which behave the same in both
// modes. The caller broadcasts unless a.noBroadcast is set.
func emitSigned(a args, signedJs []byte, txid string) error {
	if a.out != "" {
		if err := os.WriteFile(a.out, signedJs, 0600); err != nil {
			return fmt.Errorf("failed to write signed transaction to file: %w", err)
		}
		fmt.Println("wrote signed transaction to", a.out)
	}
	if a.noBroadcast {
		fmt.Println("\n===== SIGNED TRANSACTION (not broadcast) =====")
		fmt.Println(string(signedJs))
		fmt.Println("txid:", txid)
	}
	return nil
}

func readPayload(a args) (string, error) {
	raw, err := readJSONInput(a.jsonPayload, a.jsonFile)
	if err != nil {
		return "", fmt.Errorf("failed to read JSON payload: %w", err)
	}
	// Compact both validates the payload and normalizes it, so what's printed
	// for review is byte-identical to what gets signed and broadcast.
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", fmt.Errorf("payload is not valid JSON: %w", err)
	}
	return compact.String(), nil
}

// readJSONInput returns the inline value, or the contents of file when one is
// given ("-" reads stdin).
func readJSONInput(inline string, file string) ([]byte, error) {
	if file == "" {
		return []byte(inline), nil
	}
	if file == "-" {
		return readAll(os.Stdin)
	}
	return os.ReadFile(file)
}

func readAll(f *os.File) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func splitCsv(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// runExternalSigner runs the signing command via 'sh -c', feeding it the
// request on stdin and returning the validated signature hex from stdout.
// Anything the command writes to stderr is folded into the error so a
// missing/misconfigured signer surfaces a useful message.
func runExternalSigner(signCmd string, request []byte) (string, error) {
	cmd := exec.Command("sh", "-c", signCmd)
	cmd.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w\n%s", err, msg)
		}
		return "", err
	}

	sig := strings.TrimSpace(stdout.String())
	if err := validateSignatureHex(sig); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("returned an invalid signature (%w)\n%s", err, msg)
		}
		return "", fmt.Errorf("returned an invalid signature: %w", err)
	}
	return sig, nil
}

// validateSignatureHex checks the external signer returned a 65-byte compact
// recoverable secp256k1 signature in hex — the format Hive expects.
func validateSignatureHex(sig string) error {
	b, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("not valid hex: %w", err)
	}
	if len(b) != 65 {
		return fmt.Errorf("expected 65 bytes (130 hex chars), got %d bytes", len(b))
	}
	return nil
}

// wireTx is a Hive transaction in the standard wire shape, directly
// consumable by Hive tools (e.g. broadcast via condenser_api, or a signer).
type wireTx struct {
	RefBlockNum    uint16 `json:"ref_block_num"`
	RefBlockPrefix uint32 `json:"ref_block_prefix"`
	Expiration     string `json:"expiration"`
	// Operations are kept as raw JSON because Hive has two interchangeable
	// operation encodings and this tool passes through whichever one it is given.
	Operations []json.RawMessage `json:"operations"`
	Extensions []string          `json:"extensions"`
	Signatures []string          `json:"signatures"`
}

// signingBundle is the request handed to the external signer. Only
// signing_digest and path are read by the bundled ledger-signer; the rest is
// context for debugging and for other signing tools.
type signingBundle struct {
	// ChainId is folded into the digest: sha256(chainId || serialized tx).
	ChainId string `json:"chain_id"`
	// Account whose authority must sign the transaction.
	Account string `json:"account"`
	// SigningDigest is the 32-byte hash (hex) the device signs directly via the
	// Ledger Hive app's hash/blind-signing mode.
	SigningDigest string `json:"signing_digest"`
	SerializedTx  string `json:"serialized_tx"`
	TxId          string `json:"txid"`
	// Transaction is the unsigned transaction in Hive's wire shape.
	Transaction json.RawMessage `json:"transaction"`
	// Path is the BIP32 derivation path for the signer.
	Path string `json:"path"`
}

// tapos is the chain state a transaction has to pin itself to: the reference
// block that makes it valid on one fork only, and the time it stops being
// accepted.
type tapos struct {
	refBlockNum    uint16
	refBlockPrefix uint32
	expiration     string
}

// fetchTapos reads the chain head and derives the transaction's reference block
// and expiration, expirationSecs after the head block's time.
func fetchTapos(hiveClient *hivego.HiveRpcNode, expirationSecs int) (tapos, error) {
	propsB, err := hiveClient.GetDynamicGlobalProps()
	if err != nil {
		return tapos{}, fmt.Errorf("fetch dynamic global props: %w", err)
	}
	var props struct {
		HeadBlockNumber int    `json:"head_block_number"`
		HeadBlockId     string `json:"head_block_id"`
		Time            string `json:"time"`
	}
	if err := json.Unmarshal(propsB, &props); err != nil {
		return tapos{}, fmt.Errorf("parse dynamic global props: %w", err)
	}

	hbidB, err := hex.DecodeString(props.HeadBlockId)
	if err != nil || len(hbidB) < 8 {
		return tapos{}, fmt.Errorf("decode head_block_id %q: %w", props.HeadBlockId, err)
	}
	headTime, err := time.Parse("2006-01-02T15:04:05", props.Time)
	if err != nil {
		return tapos{}, fmt.Errorf("parse head block time: %w", err)
	}

	return tapos{
		refBlockNum:    uint16(props.HeadBlockNumber & 0xffff),
		refBlockPrefix: binary.LittleEndian.Uint32(hbidB[4:8]),
		expiration: headTime.
			Add(time.Duration(expirationSecs) * time.Second).
			Format("2006-01-02T15:04:05"),
	}, nil
}

// buildSigningBundle fetches chain state, builds the (unsigned) transaction
// around the custom_json operation, and returns everything the external
// signer needs. The returned hivego transaction is the canonical source for
// the digest/txid in the bundle and is what gets broadcast after AddSig.
func buildSigningBundle(
	hiveClient *hivego.HiveRpcNode,
	op hivego.CustomJsonOperation,
	a args,
) (signingBundle, hivego.HiveTransaction, error) {
	t, err := fetchTapos(hiveClient, a.expiration)
	if err != nil {
		return signingBundle{}, hivego.HiveTransaction{}, err
	}

	tx := hivego.HiveTransaction{
		RefBlockNum:    t.refBlockNum,
		RefBlockPrefix: t.refBlockPrefix,
		Expiration:     t.expiration,
		Operations:     []hivego.HiveOperation{op},
	}

	serialized, err := hivego.SerializeTx(tx)
	if err != nil {
		return signingBundle{}, hivego.HiveTransaction{}, fmt.Errorf("serialize transaction: %w", err)
	}
	digest := hivego.HashTxForSig(serialized, hiveClient.ChainID)
	txid, err := tx.GenerateTrxId()
	if err != nil {
		return signingBundle{}, hivego.HiveTransaction{}, fmt.Errorf("compute transaction id: %w", err)
	}

	opsJs, err := opsToWire([]hivego.HiveOperation{op})
	if err != nil {
		return signingBundle{}, hivego.HiveTransaction{}, fmt.Errorf("encode operations: %w", err)
	}

	wireJs, err := json.Marshal(wireTx{
		RefBlockNum:    t.refBlockNum,
		RefBlockPrefix: t.refBlockPrefix,
		Expiration:     t.expiration,
		Operations:     opsJs,
		Extensions:     []string{},
		Signatures:     []string{},
	})
	if err != nil {
		return signingBundle{}, hivego.HiveTransaction{}, fmt.Errorf("encode transaction: %w", err)
	}

	bundle := signingBundle{
		ChainId:       hiveClient.ChainID,
		Account:       a.account,
		SigningDigest: hex.EncodeToString(digest),
		SerializedTx:  hex.EncodeToString(serialized),
		TxId:          txid,
		Transaction:   wireJs,
		Path:          a.ledgerPath,
	}
	return bundle, tx, nil
}

// opsToWire encodes operations into Hive's legacy [name, obj] form.
func opsToWire(ops []hivego.HiveOperation) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(ops))
	for _, op := range ops {
		nameB, err := json.Marshal(op.OpName())
		if err != nil {
			return nil, err
		}
		objB, err := json.Marshal(op)
		if err != nil {
			return nil, err
		}
		opB, err := json.Marshal([2]json.RawMessage{nameB, objB})
		if err != nil {
			return nil, err
		}
		out = append(out, opB)
	}
	return out, nil
}
