package callcontract

import (
	"encoding/json"
	"fmt"
	"log"
	"vsc-node/lib/hive"
	"vsc-node/modules/common"
	"vsc-node/modules/db/vsc/contracts"
	"vsc-node/modules/hive/streamer"

	stateEngine "vsc-node/modules/state-processing"

	"github.com/vsc-eco/hivego"
)

const contractID = "vsc1BTpUPXMyvc6LNe38w5UNCNAURZHH6esBic"
const username = "milo-hpr"

type txVscCallContractJSON struct {
	NetId      string             `json:"net_id"`
	Caller     string             `json:"caller"`
	ContractId string             `json:"contract_id"`
	Action     string             `json:"action"`
	Payload    json.RawMessage    `json:"payload"`
	RcLimit    uint               `json:"rc_limit"`
	Intents    []contracts.Intent `json:"intents"`
}

// optional param rc limit, defaults to 1000
func CallContract(
	contractPayload json.RawMessage,
	action string,
	limit ...uint,
) error {
	identityConfig := common.NewIdentityConfig()
	identityConfig.Init()
	hiveConfig := streamer.NewHiveConfig()
	hiveConfig.Init()

	hiveRpcClient := hivego.NewHiveRpc(hiveConfig.Get().HiveURI)

	hiveCreator := hive.LiveTransactionCreator{
		TransactionCrafter: hive.TransactionCrafter{},
		TransactionBroadcaster: hive.TransactionBroadcaster{
			Client:  hiveRpcClient,
			KeyPair: identityConfig.HiveActiveKeyPair,
		},
	}

	rcLimit := uint(1000)
	if len(limit) > 0 {
		rcLimit = limit[0]
	}

	txObj := stateEngine.TxVscCallContract{
		NetId:      "vsc-mainnet",
		Caller:     fmt.Sprintf("hive:%s", username), // hive:username
		ContractId: contractID,
		Action:     action,
		Payload:    contractPayload,
		RcLimit:    rcLimit,
		Intents:    []contracts.Intent{},
	}

	wrapper := txVscCallContractJSON{
		NetId:      txObj.NetId,
		Caller:     txObj.Caller,
		ContractId: txObj.ContractId,
		Action:     txObj.Action,
		Payload:    txObj.Payload,
		RcLimit:    txObj.RcLimit,
		Intents:    txObj.Intents,
	}

	txJson, err := json.Marshal(wrapper)
	if err != nil {
		log.Printf("error marshalling txjson: %s", err.Error())
		return err
	}

	log.Println("txjson:", string(txJson))

	op := hiveCreator.CustomJson([]string{username}, []string{}, "vsc.call", string(txJson))
	tx := hiveCreator.MakeTransaction([]hivego.HiveOperation{op})

	fmt.Println("tx created")

	hiveCreator.PopulateSigningProps(&tx, nil)
	sig, err := hiveCreator.Sign(tx)
	if err != nil {
		return err
	}

	tx.AddSig(sig)
	fmt.Println("tx signed")
	_, err = hiveCreator.Broadcast(tx)
	if err != nil {
		return fmt.Errorf("error broadcasting tx: %w", err)
	}

	fmt.Println("tx broadcast")
	return nil
}
