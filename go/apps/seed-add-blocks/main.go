package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"vsc-node/modules/common"
	"vsc-node/modules/hive/streamer"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
)

const storagePath = "last_height"

type BlockSeedInput struct {
	BlockHeader string `json:"block_header"`
	BlockHeight uint32 `json:"block_height"`
}

type AddBlocksInput struct {
	Blocks    string `json:"blocks"`
	LatestFee int64  `json:"latest_fee"`
}

const MempoolTestNetBase = "https://mempool.space/testnet/api"
const MempoolBase = "https://mempool.space/api"

type MempoolClient struct {
	baseURL string
	client  *http.Client
}

func NewMempoolClient(network *string) *MempoolClient {
	var mempoolURL string
	if *network == "testnet" {
		mempoolURL = MempoolTestNetBase
	} else {
		mempoolURL = MempoolBase
	}
	return &MempoolClient{
		baseURL: mempoolURL,
		client:  &http.Client{},
	}
}

func (m *MempoolClient) GetLatestBlockHeight() (string, int, error) {
	fmt.Println("getting hash for latest block")
	url := fmt.Sprintf("%s/blocks/tip/height", m.baseURL)
	resp, err := m.client.Get(url)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("mempool API returned status %d", resp.StatusCode)
	}

	blockHash, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(blockHash), resp.StatusCode, nil
}

func (m *MempoolClient) GetBlockHashAtHeight(height uint32) (string, int, error) {
	fmt.Println("getting hash for block at height", height)
	url := fmt.Sprintf("%s/block-height/%d", m.baseURL, height)
	resp, err := m.client.Get(url)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("mempool API returned status %d", resp.StatusCode)
	}

	blockHash, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(blockHash), resp.StatusCode, nil
}

func (m *MempoolClient) GetBlockHeader(hash string) ([]byte, error) {
	fmt.Println("getting raw data for block with hash", hash)
	url := fmt.Sprintf("%s/block/%s/header", m.baseURL, hash)
	resp, err := m.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mempool API returned status %d", resp.StatusCode)
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return rawBytes, nil
}

func endCycle(input *AddBlocksInput, blockHeight uint32, nosleep ...bool) {
	if len(input.Blocks) > 0 {
		jsonPayload, err := json.Marshal(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return
		}
		err = callcontract.CallContract(jsonPayload, "add_blocks")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		input.Blocks = ""
		setLastHeight(blockHeight - 1)
	}
	if len(nosleep) == 0 || !nosleep[0] {
		fmt.Println("Sleeping")
		time.Sleep(1 * time.Minute)
	}
}

func getLastHeight() (uint32, error) {
	heightBytes, err := os.ReadFile(storagePath)
	if err != nil {
		return 0, fmt.Errorf("error reading height from file: %w", err)
	} else {
		height, err := strconv.Atoi(string(heightBytes))
		if err != nil {
			fmt.Fprintln(os.Stderr)
			return 0, fmt.Errorf("error reading number from saved height: %w", err)
		} else {
			return uint32(height), nil
		}
	}
}

func setLastHeight(height uint32) error {
	heightBytes := []byte(strconv.Itoa(int(height)))
	err := os.WriteFile(storagePath, heightBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

func gracefulShutdown(height uint32) {
	setLastHeight(height)
	os.Exit(1)
}

func main() {
	isInit := flag.Bool("init", false, "Generate credentials config files")
	network := flag.String("network", "testnet", "Which bitcoin network to fetch blocks for: testnet | mainnet")
	seed := flag.Int("seed", 0, `Block height at which to seed the. Use -1 for latest.`)
	createKey := flag.Bool("create_key", false, "Create key pair.")
	maxBlocks := flag.Uint64("max_blocks", math.MaxUint64, "Maxiumum blocks to be added.")
	flag.Parse()

	if *isInit {
		identityConfig := common.NewIdentityConfig()
		identityConfig.Init()
		hiveConfig := streamer.NewHiveConfig()
		hiveConfig.Init()
		fmt.Println("Identity config created.")
		return
	}

	if *createKey {
		err := callcontract.CallContract([]byte("{}"), "create_key_pair")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return
	}

	mempoolClient := NewMempoolClient(network)

	if *seed != 0 {
		blockHeight := uint32(*seed)
		if *seed < 0 {
			latestHeightString, status, err := mempoolClient.GetLatestBlockHeight()
			if status != http.StatusOK {
				fmt.Fprintln(os.Stderr, "Could not fetch block.")
				return
			} else if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				return
			}
			latestHeight, err := strconv.ParseUint(latestHeightString, 10, 32)
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				return
			}
			blockHeight = uint32(latestHeight)
		}
		blockHash, status, err := mempoolClient.GetBlockHashAtHeight(blockHeight)
		if status != http.StatusOK {
			fmt.Fprintln(os.Stderr, "Could not fetch block.")
			return
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return
		}

		header, err := mempoolClient.GetBlockHeader(blockHash)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error getting block header:", err.Error())
			return
		}
		seedBlocksInput := BlockSeedInput{
			BlockHeight: blockHeight,
			BlockHeader: string(header),
		}
		jsonPayload, err := json.Marshal(seedBlocksInput)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return
		}
		err = callcontract.CallContract(jsonPayload, "seed_blocks")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		} else {
			setLastHeight(blockHeight)
		}
		return
	}

	blockHeight := uint32(4737038)
	height, err := getLastHeight()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	} else {
		blockHeight = height + 1
	}

	addBlocksInput := AddBlocksInput{
		LatestFee: 1,
	}

	for i := uint64(0); i < *maxBlocks; i++ {
		hash, status, err := mempoolClient.GetBlockHashAtHeight(blockHeight)
		if status == http.StatusNotFound {
			fmt.Println("No new block.")
			endCycle(&addBlocksInput, blockHeight)
			continue
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			gracefulShutdown(blockHeight)
		}
		blockHeader, err := mempoolClient.GetBlockHeader(hash)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			gracefulShutdown(blockHeight)
		}
		addBlocksInput.Blocks += string(blockHeader)
		blockHeight++

		if len(addBlocksInput.Blocks) > 4000 {
			endCycle(&addBlocksInput, blockHeight)
			continue
		}
	}

	endCycle(&addBlocksInput, blockHeight, true)
}
