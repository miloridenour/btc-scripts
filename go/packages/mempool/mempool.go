package mempool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	MempoolAPITestnet  = "https://mempool.space/testnet/api"
	MempoolAPITestnet4 = "https://mempool.space/testnet4/api"
	MempoolAPIMainnet  = "https://mempool.space/api"
)

type MempoolClient struct {
	baseURL string
	client  *http.Client
}

type TxOutput struct {
	ScriptPubKey        string `json:"scriptpubkey"`
	ScriptPubKeyAsm     string `json:"scriptpubkey_asm"`
	ScriptPubKeyType    string `json:"scriptpubkey_type"`
	ScriptPubKeyAddress string `json:"scriptpubkey_address"`
	Value               int64  `json:"value"`
}

type TxStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int64  `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	BlockTime   int64  `json:"block_time"`
}

type TxInfo struct {
	TxId   string     `json:"txid"`
	Vout   []TxOutput `json:"vout"`
	Status TxStatus   `json:"status"`
}

func BaseURLForNetwork(network string) string {
	switch network {
	case "mainnet":
		return MempoolAPIMainnet
	case "testnet":
		return MempoolAPITestnet
	default:
		return MempoolAPITestnet4
	}
}

func NewMempoolClient(httpClient *http.Client, baseURL string) *MempoolClient {
	return &MempoolClient{
		baseURL: baseURL,
		client:  httpClient,
	}
}

// GetTx fetches full transaction info from mempool.space
func (m *MempoolClient) GetTx(txid string) (*TxInfo, error) {
	url := fmt.Sprintf("%s/tx/%s", m.baseURL, txid)
	resp, err := m.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tx: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mempool API returned %d: %s", resp.StatusCode, string(body))
	}

	var txInfo TxInfo
	if err := json.NewDecoder(resp.Body).Decode(&txInfo); err != nil {
		return nil, fmt.Errorf("failed to decode tx response: %w", err)
	}

	return &txInfo, nil
}

func (m *MempoolClient) PostTx(rawTx string) error {
	url := fmt.Sprintf("%s/tx", m.baseURL)
	resp, err := m.client.Post(url, "text/plain", bytes.NewReader([]byte(rawTx)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("broadcast failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
