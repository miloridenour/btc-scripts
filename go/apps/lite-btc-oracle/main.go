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
	"sync"
	"time"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

const storageDir = "last-height"
const storageFile = "last_height"
const storagePath = storageDir + "/" + storageFile

type BlockSeedInput struct {
	BlockHeader string `json:"block_header"`
	BlockHeight uint32 `json:"block_height"`
}

type AddBlocksInput struct {
	Blocks    string `json:"blocks"`
	LatestFee int64  `json:"latest_fee"`
}

const MempoolTestNetBase = "https://mempool.space/testnet/api"
const MempoolTestNet4Base = "https://mempool.space/testnet4/api"
const MempoolBase = "https://mempool.space/api"

type MempoolClient struct {
	baseURL string
	client  *http.Client
}

type Config struct {
	HiveActiveKey string `json:"HiveActiveKey"`
	HiveUsername  string `json:"HiveUsername"`
	HiveURI       string `json:"HiveURI"`
	HiveChainID   string `json:"HiveChainID"`
	VscNetID      string `json:"VscNetID"`
	ContractID    string `json:"ContractID"`
}

func NewMempoolClient(network *string) *MempoolClient {
	var mempoolURL string
	switch *network {
	case "testnet":
		mempoolURL = MempoolTestNetBase
	case "testnet4":
		mempoolURL = MempoolTestNet4Base
	default:
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

var hiveConfig callcontract.HiveConfig

type OracleControl struct {
	mu          sync.Mutex
	running     bool
	blockHeight uint32
	wake        chan struct{}
}

func NewOracleControl(startHeight uint32) *OracleControl {
	return &OracleControl{
		running:     true,
		blockHeight: startHeight,
		wake:        make(chan struct{}, 1),
	}
}

func (oc *OracleControl) IsRunning() bool {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.running
}

func (oc *OracleControl) Stop() {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.running = false
}

func (oc *OracleControl) Start() {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.running = true
	select {
	case oc.wake <- struct{}{}:
	default:
	}
}

func (oc *OracleControl) GetHeight() uint32 {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.blockHeight
}

func (oc *OracleControl) SetHeight(h uint32) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.blockHeight = h
	setLastHeight(h)
}

func startControlServer(oc *OracleControl, port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		oc.Stop()
		fmt.Fprintln(w, "stopped")
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		oc.Start()
		fmt.Fprintln(w, "started")
	})

	mux.HandleFunc("/set-height", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Height uint32 `json:"height"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		oc.SetHeight(body.Height)
		fmt.Fprintf(w, "height set to %d\n", body.Height)
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"running": oc.IsRunning(),
			"height":  oc.GetHeight(),
		})
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Println("control server listening on", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "control server error:", err)
	}
}

func endCycle(input *AddBlocksInput, blockHeight uint32, nosleep ...bool) {
	if len(input.Blocks) > 0 {
		jsonPayload, err := json.Marshal(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return
		}
		err = callcontract.CallContract(hiveConfig, jsonPayload, "addBlocks", 10000)
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
	network := flag.String(
		"network",
		"testnet4",
		"Which bitcoin network to fetch blocks for: testnet | testnet4 | mainnet",
	)
	seed := flag.Int("seed", 0, `Block height at which to seed the. Use -1 for latest.`)
	createKey := flag.Bool("create-key", false, "Create key pair.")
	hardStopBlocks := flag.Uint64(
		"hard-stop-after-blocks",
		math.MaxUint64,
		"Maxiumum blocks to be added before terminating script (usually don't use).",
	)
	blocksPerTx := flag.Int("blocks-per-tx", 64, "Maxiumum blocks to be added per transaction.")
	controlPort := flag.Int("port", 8080, "Port for the HTTP control server.")
	flag.Parse()

	config := Config{}

	inputconfig.LoadConfig(&config)

	if *isInit {
		inputconfig.SaveConfig(config)
		setLastHeight(0)
		return
	}

	if config.HiveActiveKey == "" || config.HiveUsername == "" || config.HiveURI == "" {
		fmt.Fprintln(os.Stderr, "config not initialized")
		os.Exit(1)
	}

	hiveConfig = callcontract.HiveConfig{
		ActiveKey:  config.HiveActiveKey,
		Username:   config.HiveUsername,
		URI:        config.HiveURI,
		ChainID:    config.HiveChainID,
		VscNetID:   config.VscNetID,
		ContractID: config.ContractID,
	}

	if *createKey {
		err := callcontract.CallContract(hiveConfig, []byte("{}"), "createKeyPair")
		if err != nil {
			fmt.Fprintln(os.Stderr, "createKeyPair failed:", err.Error())
			os.Exit(1)
		}
		return
	}

	mempoolClient := NewMempoolClient(network)
	fmt.Println("using btc api:", mempoolClient.baseURL)

	if *seed != 0 {
		blockHeight := uint32(*seed)
		if *seed < 0 {
			latestHeightString, status, err := mempoolClient.GetLatestBlockHeight()
			if status != http.StatusOK {
				fmt.Fprintln(os.Stderr, "could not fetch latest block height: status", status)
				os.Exit(1)
			} else if err != nil {
				fmt.Fprintln(os.Stderr, "could not fetch latest block height:", err.Error())
				os.Exit(1)
			}
			latestHeight, err := strconv.ParseUint(latestHeightString, 10, 32)
			if err != nil {
				fmt.Fprintln(os.Stderr, "could not parse latest block height:", err.Error())
				os.Exit(1)
			}
			blockHeight = uint32(latestHeight)
		}
		blockHash, status, err := mempoolClient.GetBlockHashAtHeight(blockHeight)
		if status != http.StatusOK {
			fmt.Fprintln(os.Stderr, "could not fetch block hash at height", blockHeight, ": status", status)
			os.Exit(1)
		} else if err != nil {
			fmt.Fprintln(os.Stderr, "could not fetch block hash at height", blockHeight, ":", err.Error())
			os.Exit(1)
		}

		header, err := mempoolClient.GetBlockHeader(blockHash)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error getting block header:", err.Error())
			os.Exit(1)
		}
		seedBlocksInput := BlockSeedInput{
			BlockHeight: blockHeight,
			BlockHeader: string(header),
		}
		jsonPayload, err := json.Marshal(seedBlocksInput)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error marshaling seed payload:", err.Error())
			os.Exit(1)
		}
		err = callcontract.CallContract(hiveConfig, jsonPayload, "seedBlocks")
		if err != nil {
			fmt.Fprintln(os.Stderr, "seedBlocks contract call failed:", err.Error())
			os.Exit(1)
		}
		setLastHeight(blockHeight)
		return
	}

	blockHeight := uint32(4737038)
	height, err := getLastHeight()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	} else {
		blockHeight = height + 1
	}

	oc := NewOracleControl(blockHeight)
	go startControlServer(oc, *controlPort)

	addBlocksInput := AddBlocksInput{
		LatestFee: 1,
	}

	blocksInTx := 0
	for i := uint64(0); i < *hardStopBlocks; i++ {
		for !oc.IsRunning() {
			fmt.Println("Paused, waiting for start...")
			<-oc.wake
		}
		blockHeight = oc.GetHeight()

		hash, status, err := mempoolClient.GetBlockHashAtHeight(blockHeight)
		if status == http.StatusNotFound {
			fmt.Println("No new block.")
			endCycle(&addBlocksInput, blockHeight)
			blocksInTx = 0
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
		oc.SetHeight(blockHeight)
		blocksInTx++

		if blocksInTx >= *blocksPerTx {
			endCycle(&addBlocksInput, blockHeight)
			blocksInTx = 0
			continue
		}
	}

	fmt.Println("max_blocks limit reached, exiting")
	endCycle(&addBlocksInput, blockHeight, true)
}
