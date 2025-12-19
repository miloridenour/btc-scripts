package mempool

import (
	"bytes"
	"fmt"
	"net/http"
)

const MempoolAPIBase = "https://mempool.space/testnet/api"

type MempoolClient struct {
	baseURL string
	client  *http.Client
}

func NewMempoolClient(httpClient *http.Client) *MempoolClient {
	return &MempoolClient{
		baseURL: MempoolAPIBase,
		client:  httpClient,
	}
}

func (m *MempoolClient) PostTx(rawTx string) error {
	url := fmt.Sprintf("%s/tx", m.baseURL)
	resp, err := m.client.Post(url, "test/plain", bytes.NewReader([]byte(rawTx)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
