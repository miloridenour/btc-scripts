package main

import (
	"testing"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
)

var testHiveConfig = callcontract.HiveConfig{}

func TestSendUnmapTx(t *testing.T) {
	callcontract.CallContract(
		testHiveConfig,
		[]byte(`{"amount":8000,"recipient_btc_address":"tb1q5dgehs94wf5mgfasnfjsh4dqv6hz8e35w4w7tk"}`),
		"unmap",
	)
}

func TestRegisterKey(t *testing.T) {
	callcontract.CallContract(
		testHiveConfig,
		[]byte(`"0324e4058be50b8b584f8bbcb4dbbeca5145bada0baba657354c56d45a4ef0bd02"`),
		"register_public_key",
	)
}
