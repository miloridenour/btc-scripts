// swap-bot sends repeated swap transactions across all combinations of a
// configured asset list against a DEX router contract.
//
// It is a general test driver (not a flood test): each round it broadcasts a
// small batch of 1-3 swaps, cycling through every ordered asset pair (both
// directions), then waits and repeats. The router decides whether a given pair
// is a direct (1-hop) or two-hop swap, so the bot exercises both transparently.
//
// Authorization differs by input asset:
//   - Native input (HIVE/HBD): the swap "execute" call carries a "transfer.allow"
//     intent letting the router draw asset_in from the caller's ledger balance.
//   - Mapped input (BTC, ...): a separate "increaseAllowance" call is broadcast
//     to the asset's mapping contract first (approving the router as spender),
//     then the swap "execute" is broadcast with no intents (the router pulls via
//     the ERC-20-style allowance). This mirrors the production altera-app client.
//
// Each swap's input amount is drawn uniformly at random from the asset's
// configured [min_amount, max_amount] range, so swap sizes vary across the run.
//
// Balancing ledger: the bot tracks the total input volume it has pushed in each
// direction of each pair. A final reconciliation round issues the offsetting
// swaps needed to even out the two directions of every pair, so each underlying
// pool ends at roughly its starting reserves (minus accumulated swap fees). A
// swap X->Y and its reverse Y->X traverse the same pool(s) in opposite
// directions, so matched volume per direction restores each pool — provided
// per-asset amounts are configured to be roughly equal in *value* (so a round
// trip returns ~what it took). Because sizes are random, volume is normalized by
// each asset's mean swap size (the midpoint of its range) when comparing the two
// directions. See -no-rebalance to skip the reconciliation round.
//
// Configuration (Hive credentials, router, and the asset list) lives in
// data/inputs/scriptInputs.json. Run once with -init to scaffold a template,
// fill it in, then run for real.
package main

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miloridenour/vsc-scripts/packages/callcontract"
	"github.com/miloridenour/vsc-scripts/packages/inputconfig"
)

const swapVersion = "1.0.0"

// nativeAssets are settled directly on the ledger and use a transfer.allow
// intent rather than a mapping-contract allowance.
var nativeAssets = map[string]bool{"HIVE": true, "HBD": true}

// AssetConfig describes one tradable asset in the swap set.
type AssetConfig struct {
	// Symbol is the asset symbol, e.g. "HIVE", "HBD", "BTC". Case-insensitive.
	Symbol string `json:"symbol"`
	// MinAmount and MaxAmount bound the per-swap amount_in for this asset, in its
	// smallest unit (decimal strings). Each swap picks a uniform random value in
	// [MinAmount, MaxAmount]. Choose ranges of roughly equal value across assets
	// so round-trip swaps restore pool reserves. Set both equal for a fixed size.
	MinAmount string `json:"min_amount"`
	MaxAmount string `json:"max_amount"`
	// MappingContract is the mapping (ERC-20) contract ID for a mapped asset.
	// Leave empty for native assets (HIVE/HBD).
	MappingContract string `json:"mapping_contract,omitempty"`
}

// asset is a validated, parsed AssetConfig used at runtime.
type asset struct {
	symbol  string
	min     int64
	max     int64
	mean    float64 // midpoint of [min, max], used to normalize volume across assets
	mapping string
}

// Config mirrors sign-load-test's credential layout and adds the router and
// asset set this bot trades.
type Config struct {
	HiveActiveKey  string        `json:"HiveActiveKey"`
	HiveUsername   string        `json:"HiveUsername"`
	HiveURI        string        `json:"HiveURI"`
	HiveChainID    string        `json:"HiveChainID"`
	VscNetID       string        `json:"VscNetID"`
	RouterContract string        `json:"RouterContract"`
	Assets         []AssetConfig `json:"Assets"`
}

// SwapInstruction is the router "execute" payload for a swap. String amounts are
// in the asset's smallest unit.
type SwapInstruction struct {
	Type         string `json:"type"`
	Version      string `json:"version"`
	AssetIn      string `json:"asset_in"`
	AssetOut     string `json:"asset_out"`
	AmountIn     string `json:"amount_in"`
	MinAmountOut string `json:"min_amount_out"`
	Recipient    string `json:"recipient"`
}

// allowanceParams is the increaseAllowance payload for a mapping contract.
type allowanceParams struct {
	Spender string `json:"spender"`
	Amount  string `json:"amount"`
}

// route is one directed swap (asset_in -> asset_out).
type route struct{ In, Out string }

func main() {
	n := flag.Int("n", 0, "number of normal swap rounds to run (required unless -init)")
	batchMin := flag.Int("batch-min", 1, "minimum swaps per round")
	batchMax := flag.Int("batch-max", 3, "maximum swaps per round")
	interval := flag.Duration("interval", 30*time.Second, "delay between rounds")
	intraDelay := flag.Duration("intra-delay", 1*time.Second, "delay between broadcasts within a round")
	minAmountOut := flag.String("min-amount-out", "0", "min_amount_out (slippage floor) sent on every swap")
	recipient := flag.String(
		"recipient",
		"",
		"recipient for swap output; defaults to hive:<username> (keep self to recycle funds)",
	)
	swapRC := flag.Uint("rc-limit", 10000, "RC limit per swap execute call")
	allowanceRC := flag.Uint("allowance-rc-limit", 1000, "RC limit per increaseAllowance call")
	noRebalance := flag.Bool("no-rebalance", false, "skip the final reconciliation round")
	dryRun := flag.Bool("dry-run", false, "log and track swaps without broadcasting any transactions")
	routerOverride := flag.String("router", "", "override the router contract ID from config")
	init := flag.Bool("init", false, "scaffold a template config file and exit")

	flag.Parse()

	if *init {
		scaffoldConfig()
		return
	}

	if *n <= 0 {
		log.Fatal("-n must be > 0")
	}
	if *batchMin < 1 || *batchMax < *batchMin {
		log.Fatal("require 1 <= -batch-min <= -batch-max")
	}

	var cfg Config
	if err := inputconfig.LoadConfig(&cfg); err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Println("config file created at data/inputs/scriptInputs.json; fill it in and re-run")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}

	if *routerOverride != "" {
		cfg.RouterContract = *routerOverride
	}
	if len(cfg.RouterContract) != 38 || !strings.HasPrefix(cfg.RouterContract, "vsc1") {
		log.Fatal("RouterContract must be a valid contract ID (vsc1...)")
	}

	assets, order := buildAssetMap(cfg.Assets, cfg.RouterContract)

	rcpt := *recipient
	if rcpt == "" {
		rcpt = "hive:" + cfg.HiveUsername
	}

	hiveConfig := callcontract.HiveConfig{
		ActiveKey:  cfg.HiveActiveKey,
		Username:   cfg.HiveUsername,
		URI:        cfg.HiveURI,
		ChainID:    cfg.HiveChainID,
		VscNetID:   cfg.VscNetID,
		ContractID: cfg.RouterContract,
	}

	// All ordered pairs (both directions) = the full combination set.
	routes := allRoutes(order)
	rng := newRNG()

	log.Printf("swap-bot: %d assets, %d routes (combinations), %d rounds, %d-%d swaps/round, recipient=%s, router=%s",
		len(order), len(routes), *n, *batchMin, *batchMax, rcpt, cfg.RouterContract)
	for _, r := range routes {
		log.Printf("  route %s -> %s", r.In, r.Out)
	}

	bot := &swapper{
		hive:        hiveConfig,
		assets:      assets,
		rng:         rng,
		recipient:   rcpt,
		minOut:      *minAmountOut,
		swapRC:      *swapRC,
		allowanceRC: *allowanceRC,
		intraDelay:  *intraDelay,
		dryRun:      *dryRun,
		volume:      map[route]int64{},
		counts:      map[route]int{},
	}

	// queue is a reshuffling cycle over all routes so every combination is
	// exercised evenly across the run.
	queue := append([]route(nil), routes...)
	rng.Shuffle(len(queue), func(i, j int) { queue[i], queue[j] = queue[j], queue[i] })
	span := *batchMax - *batchMin + 1

	for round := 0; round < *n; round++ {
		batch := *batchMin + rng.Intn(span)
		log.Printf("=== round %d/%d: %d swap(s) ===", round+1, *n, batch)
		for j := 0; j < batch; j++ {
			if len(queue) == 0 {
				queue = append(queue, routes...)
				rng.Shuffle(len(queue), func(i, k int) { queue[i], queue[k] = queue[k], queue[i] })
			}
			r := queue[0]
			queue = queue[1:]
			bot.doSwap(r)
			bot.maybeDelay()
		}
		if (round != *n-1 || !*noRebalance) && !*dryRun {
			time.Sleep(*interval)
		}
	}

	if !*noRebalance {
		bot.reconcile(order)
	}

	bot.summary(order)
	if bot.failed > 0 {
		os.Exit(1)
	}
}

// swapper holds run state and the balancing ledger.
type swapper struct {
	hive        callcontract.HiveConfig
	assets      map[string]asset
	rng         *rand.Rand
	recipient   string
	minOut      string
	swapRC      uint
	allowanceRC uint
	intraDelay  time.Duration
	dryRun      bool

	// volume is the balancing ledger: total input amount pushed in each
	// direction, in the input asset's smallest unit. counts is informational.
	volume       map[route]int64
	counts       map[route]int
	sent, failed int
	pendingDelay bool
}

// randAmount returns a uniform random per-swap amount in the asset's range.
func (s *swapper) randAmount(a asset) int64 {
	if a.max <= a.min {
		return a.min
	}
	return a.min + s.rng.Int63n(a.max-a.min+1)
}

func (s *swapper) maybeDelay() {
	if s.pendingDelay && !s.dryRun {
		time.Sleep(s.intraDelay)
	}
	s.pendingDelay = true
}

// doSwap broadcasts one swap for the given route. For a mapped asset_in it
// bundles an "increaseAllowance" op ahead of the "execute" op in a single
// transaction, so the allowance is applied before the swap's transferFrom (the
// ops are ordered and atomic within one tx, matching the production client).
func (s *swapper) doSwap(r route) {
	in := s.assets[r.In]
	amount := s.randAmount(in)
	amountStr := strconv.FormatInt(amount, 10)

	instr := SwapInstruction{
		Type:         "swap",
		Version:      swapVersion,
		AssetIn:      r.In,
		AssetOut:     r.Out,
		AmountIn:     amountStr,
		MinAmountOut: s.minOut,
		Recipient:    s.recipient,
	}
	swapPayload, err := json.Marshal(instr)
	if err != nil {
		log.Fatalf("marshal swap instruction: %s", err.Error())
	}

	var calls []callcontract.ContractCall

	if nativeAssets[r.In] {
		// Native input authorizes the draw via a transfer.allow intent.
		calls = append(calls, callcontract.ContractCall{
			Action:  "execute",
			Payload: swapPayload,
			RcLimit: s.swapRC,
			Intents: []callcontract.Intent{{
				Type: "transfer.allow",
				Args: map[string]string{"token": strings.ToLower(r.In), "limit": amountStr},
			}},
		})
		fmt.Printf("[swap] %s %s -> %s (min_out=%s): %s\n", amountStr, r.In, r.Out, s.minOut, string(swapPayload))
	} else {
		// Mapped input: approve the router on the mapping contract, then swap.
		// The router pulls via the allowance, so the execute op carries no intent.
		allowPayload, err := json.Marshal(allowanceParams{
			Spender: "contract:" + s.hive.ContractID,
			Amount:  amountStr,
		})
		if err != nil {
			log.Fatalf("marshal increaseAllowance: %s", err.Error())
		}
		calls = append(calls,
			callcontract.ContractCall{
				ContractID: in.mapping,
				Action:     "increaseAllowance",
				Payload:    allowPayload,
				RcLimit:    s.allowanceRC,
			},
			callcontract.ContractCall{
				Action:  "execute",
				Payload: swapPayload,
				RcLimit: s.swapRC,
			},
		)
		fmt.Printf("[swap] %s %s -> %s (min_out=%s) [+increaseAllowance %s on %s]: %s\n",
			amountStr, r.In, r.Out, s.minOut, amountStr, in.mapping, string(swapPayload))
	}

	if s.dryRun {
		fmt.Printf("[dry-run] would broadcast %d op(s) for %s -> %s\n", len(calls), r.In, r.Out)
	} else if err := callcontract.CallContractMulti(s.hive, calls); err != nil {
		s.failed++
		fmt.Fprintf(os.Stderr, "swap %s->%s failed: %s\n", r.In, r.Out, err.Error())
		return
	}
	s.sent++
	s.counts[r]++
	s.volume[r] += amount
}

// reconcile issues the offsetting swaps needed to even out the two directions of
// every pair, returning each pool to ~its starting reserves (minus fees). Since
// swap sizes are random, it compares directions by volume normalized to each
// asset's mean swap size, then issues that many (mean-sized) offsetting swaps in
// the deficient direction. Each offsetting swap is itself randomly sized, so the
// result is approximate — close enough for the "about the same minus fees" goal.
func (s *swapper) reconcile(order []string) {
	var todo []route
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			a, b := order[i], order[j]
			// Normalized volume = how many mean-sized swaps' worth went each way.
			unitsAB := float64(s.volume[route{a, b}]) / s.assets[a].mean
			unitsBA := float64(s.volume[route{b, a}]) / s.assets[b].mean
			gap := unitsAB - unitsBA
			// More value sent a->b than b->a: offset with b->a swaps (and vice versa).
			off := route{b, a}
			if gap < 0 {
				gap, off = -gap, route{a, b}
			}
			for k := 0; k < int(math.Round(gap)); k++ {
				todo = append(todo, off)
			}
		}
	}

	if len(todo) == 0 {
		log.Print("=== rebalance round: already balanced, nothing to do ===")
		return
	}
	log.Printf("=== rebalance round: %d offsetting swap(s) to restore pools ===", len(todo))
	for _, r := range todo {
		s.doSwap(r)
		s.maybeDelay()
	}
}

func (s *swapper) summary(order []string) {
	log.Printf("done: %d swaps sent, %d failed", s.sent, s.failed)
	log.Print("directed volume (balancing ledger): in=count@volume, residual in mean-swap units:")
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			a, b := order[i], order[j]
			cAB, cBA := s.counts[route{a, b}], s.counts[route{b, a}]
			vAB, vBA := s.volume[route{a, b}], s.volume[route{b, a}]
			if cAB == 0 && cBA == 0 {
				continue
			}
			residual := float64(vAB)/s.assets[a].mean - float64(vBA)/s.assets[b].mean
			status := "balanced"
			if math.Abs(residual) >= 1 {
				status = "IMBALANCED"
			}
			log.Printf("  %s<->%s: %s->%s %d@%d, %s->%s %d@%d, residual %+.2f (%s)",
				a, b, a, b, cAB, vAB, b, a, cBA, vBA, residual, status)
		}
	}
}

// buildAssetMap validates and parses the configured assets, returning a lookup
// keyed by uppercase symbol and the original declared order.
func buildAssetMap(in []AssetConfig, router string) (map[string]asset, []string) {
	if len(in) < 2 {
		log.Fatal("Assets must list at least 2 assets")
	}
	m := make(map[string]asset, len(in))
	order := make([]string, 0, len(in))
	for _, a := range in {
		sym := strings.ToUpper(strings.TrimSpace(a.Symbol))
		if sym == "" {
			log.Fatal("asset symbol must not be empty")
		}
		if _, dup := m[sym]; dup {
			log.Fatalf("duplicate asset %q", sym)
		}
		min, err := strconv.ParseInt(strings.TrimSpace(a.MinAmount), 10, 64)
		if err != nil || min <= 0 {
			log.Fatalf("asset %s: min_amount %q must be a positive integer (smallest unit)", sym, a.MinAmount)
		}
		max, err := strconv.ParseInt(strings.TrimSpace(a.MaxAmount), 10, 64)
		if err != nil || max < min {
			log.Fatalf("asset %s: max_amount %q must be an integer >= min_amount", sym, a.MaxAmount)
		}
		parsed := asset{symbol: sym, min: min, max: max, mean: float64(min+max) / 2, mapping: a.MappingContract}
		if !nativeAssets[sym] {
			if len(a.MappingContract) != 38 || !strings.HasPrefix(a.MappingContract, "vsc1") {
				log.Fatalf("mapped asset %s requires a valid mapping_contract (vsc1...)", sym)
			}
			if a.MappingContract == router {
				log.Fatalf("asset %s mapping_contract must not be the router", sym)
			}
		}
		m[sym] = parsed
		order = append(order, sym)
	}
	return m, order
}

// allRoutes returns every ordered pair (both directions) of the assets.
func allRoutes(order []string) []route {
	var routes []route
	for _, a := range order {
		for _, b := range order {
			if a != b {
				routes = append(routes, route{a, b})
			}
		}
	}
	return routes
}

// scaffoldConfig writes a template config (if none exists) showing the expected
// shape with native and mapped example assets.
func scaffoldConfig() {
	template := Config{
		HiveURI:        "https://api.hive.blog",
		HiveChainID:    "beeab0de00000000000000000000000000000000000000000000000000000000",
		RouterContract: "vsc1...router...",
		Assets: []AssetConfig{
			{Symbol: "HIVE", MinAmount: "250", MaxAmount: "750"},
			{Symbol: "HBD", MinAmount: "250", MaxAmount: "750"},
			{Symbol: "BTC", MinAmount: "1000", MaxAmount: "3000", MappingContract: "vsc1...btc-mapping..."},
		},
	}
	if err := inputconfig.LoadConfig(&template); err != nil {
		if err == inputconfig.ErrConfigNotFound {
			fmt.Println("template config written to data/inputs/scriptInputs.json; fill it in and re-run")
			return
		}
		log.Fatalf("error loading config: %s", err.Error())
	}
	fmt.Println("config already exists at data/inputs/scriptInputs.json")
}

// newRNG returns a math/rand source seeded from crypto/rand so batch sizes and
// route order vary across runs without the disallowed time-based or global seeds.
func newRNG() *rand.Rand {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		log.Fatalf("error seeding rng: %s", err.Error())
	}
	return rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(b[:]))))
}
