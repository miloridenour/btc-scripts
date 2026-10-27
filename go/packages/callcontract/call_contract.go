package callcontract

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/vsc-eco/hivego"
)

type HiveConfig struct {
	ActiveKey  string
	Username   string
	URI        string
	ChainID    string
	VscNetID   string
	ContractID string
}

// Intent mirrors the on-chain intent shape (e.g. "transfer.allow") expected by
// contracts that draw funds from the caller's ledger balance, such as DEX swaps.
type Intent struct {
	Type string            `json:"type"`
	Args map[string]string `json:"args"`
}

type txVscCallContractJSON struct {
	NetId      string          `json:"net_id"`
	Caller     string          `json:"caller"`
	ContractId string          `json:"contract_id"`
	Action     string          `json:"action"`
	Payload    json.RawMessage `json:"payload"`
	RcLimit    uint            `json:"rc_limit"`
	Intents    []Intent        `json:"intents"`
}

// CallContract broadcasts a vsc.call with no intents.
// optional param rc limit, defaults to 1000
func CallContract(
	hiveConfig HiveConfig,
	contractPayload json.RawMessage,
	action string,
	limit ...uint,
) error {
	return CallContractWithIntents(hiveConfig, contractPayload, action, nil, limit...)
}

// CallContractWithIntents broadcasts a vsc.call including the given intents.
// Intents (e.g. "transfer.allow") authorize the contract to draw funds from the
// caller's ledger balance and are required for operations such as DEX swaps.
// optional param rc limit, defaults to 1000
func CallContractWithIntents(
	hiveConfig HiveConfig,
	contractPayload json.RawMessage,
	action string,
	intents []Intent,
	limit ...uint,
) error {
	username := hiveConfig.Username
	activeKey := hiveConfig.ActiveKey

	hiveRpcClient := hivego.NewHiveRpc([]string{hiveConfig.URI})
	hiveRpcClient.ChainID = hiveConfig.ChainID

	rcLimit := uint(1000)
	if len(limit) > 0 {
		rcLimit = limit[0]
	}

	if intents == nil {
		intents = []Intent{}
	}

	wrapper := txVscCallContractJSON{
		NetId:      hiveConfig.VscNetID,
		Caller:     fmt.Sprintf("hive:%s", username),
		ContractId: hiveConfig.ContractID,
		Action:     action,
		Payload:    contractPayload,
		RcLimit:    rcLimit,
		Intents:    intents,
	}

	txJson, err := json.Marshal(wrapper)
	if err != nil {
		log.Printf("error marshalling txjson: %s", err.Error())
		return err
	}

	log.Println("txjson:", string(txJson))

	txId, err := hiveRpcClient.BroadcastJson(
		[]string{username}, []string{}, "vsc.call", string(txJson), &activeKey,
	)
	if err != nil {
		return fmt.Errorf("error broadcasting tx: %w", err)
	}

	fmt.Println("tx broadcast, id:", txId)
	return nil
}

// ContractCall describes a single vsc.call op for inclusion in a multi-op
// broadcast via CallContractMulti.
type ContractCall struct {
	// ContractID is the target contract. Defaults to HiveConfig.ContractID when empty.
	ContractID string
	Action     string
	Payload    json.RawMessage
	Intents    []Intent
	// RcLimit defaults to 1000 when zero.
	RcLimit uint
}

// CallContractMulti broadcasts several vsc.call ops in a single Hive
// transaction, preserving their order. This matches how clients bundle, for
// example, an "increaseAllowance" op immediately followed by the "execute" swap
// op so the allowance is in place before the swap's transferFrom runs (the two
// ops are applied atomically and in order within one transaction).
func CallContractMulti(hiveConfig HiveConfig, calls []ContractCall) error {
	if len(calls) == 0 {
		return fmt.Errorf("no calls to broadcast")
	}

	hiveRpcClient := hivego.NewHiveRpc([]string{hiveConfig.URI})
	hiveRpcClient.ChainID = hiveConfig.ChainID

	ops := make([]hivego.HiveOperation, 0, len(calls))
	for _, c := range calls {
		contractID := c.ContractID
		if contractID == "" {
			contractID = hiveConfig.ContractID
		}
		rcLimit := c.RcLimit
		if rcLimit == 0 {
			rcLimit = 1000
		}
		intents := c.Intents
		if intents == nil {
			intents = []Intent{}
		}

		wrapper := txVscCallContractJSON{
			NetId:      hiveConfig.VscNetID,
			Caller:     fmt.Sprintf("hive:%s", hiveConfig.Username),
			ContractId: contractID,
			Action:     c.Action,
			Payload:    c.Payload,
			RcLimit:    rcLimit,
			Intents:    intents,
		}
		txJson, err := json.Marshal(wrapper)
		if err != nil {
			return fmt.Errorf("error marshalling txjson: %w", err)
		}
		log.Println("txjson:", string(txJson))

		ops = append(ops, hivego.CustomJsonOperation{
			RequiredAuths:        []string{hiveConfig.Username},
			RequiredPostingAuths: []string{},
			Id:                   "vsc.call",
			Json:                 string(txJson),
		})
	}

	activeKey := hiveConfig.ActiveKey
	txId, err := hiveRpcClient.Broadcast(ops, &activeKey)
	if err != nil {
		return fmt.Errorf("error broadcasting tx: %w", err)
	}

	fmt.Println("tx broadcast, id:", txId)
	return nil
}
