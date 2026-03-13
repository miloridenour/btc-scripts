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

type txVscCallContractJSON struct {
	NetId      string          `json:"net_id"`
	Caller     string          `json:"caller"`
	ContractId string          `json:"contract_id"`
	Action     string          `json:"action"`
	Payload    json.RawMessage `json:"payload"`
	RcLimit    uint            `json:"rc_limit"`
	Intents    []interface{}   `json:"intents"`
}

// optional param rc limit, defaults to 1000
func CallContract(
	hiveConfig HiveConfig,
	contractPayload json.RawMessage,
	action string,
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

	wrapper := txVscCallContractJSON{
		NetId:      hiveConfig.VscNetID,
		Caller:     fmt.Sprintf("hive:%s", username),
		ContractId: hiveConfig.ContractID,
		Action:     action,
		Payload:    contractPayload,
		RcLimit:    rcLimit,
		Intents:    []any{},
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
