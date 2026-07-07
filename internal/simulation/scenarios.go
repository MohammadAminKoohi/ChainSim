package simulation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/core"
	"github.com/MohammadAminKoohi/ChainSim/internal/miner"
	"github.com/MohammadAminKoohi/ChainSim/internal/network"
)

const resultsDir = "results"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupEnvironment(delta time.Duration, logFilename string) (*core.BlockChain, *network.Network, chan *core.Block, *MetricsCollector) {
	genesis := &core.Block{
		Header: core.BlockHeader{Difficulty: 1.0, Timestamp: 1719878400000}, // Fixed timestamp for determinism
		Body:   "Genesis",
	}
	chain, _ := core.NewBlockChain(genesis)
	net := network.NewNetwork(delta)
	outbound := make(chan *core.Block, 1000)

	mc, err := NewMetricsCollector(resultsDir, logFilename)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize metrics collector for %s: %s", logFilename, err.Error()))
	}

	return chain, net, outbound, mc
}

// startRouter reads mined blocks from outbound, logs them, and broadcasts to the network.
func startRouter(ctx context.Context, wg *sync.WaitGroup, net *network.Network, outbound <-chan *core.Block, mc *MetricsCollector) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-outbound:
				senderID, height := parseBlockBody(b.Body)
				mc.LogBlockMined(senderID, b.Hash(), b.Header.Difficulty, height)
				net.Broadcast(b, senderID)
			}
		}
	}()
}

// parseBlockBody extracts the miner ID and height from the body string.
// Format: "Mined by <ID> at height <N>"
func parseBlockBody(body string) (minerID string, height uint64) {
	minerID = "unknown"
	parts := strings.Fields(body)
	if len(parts) >= 6 && parts[0] == "Mined" && parts[1] == "by" && parts[3] == "at" && parts[4] == "height" {
		minerID = parts[2]
		fmt.Sscanf(parts[5], "%d", &height)
	}
	return
}

// waitForHeight polls the chain until the heaviest tip reaches the target height.
func waitForHeight(ctx context.Context, chain *core.BlockChain, target uint64, mc *MetricsCollector) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var lastTip string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tip := chain.GetHeaviestChainTip()
			if tip != nil {
				if tip.Hash != lastTip {
					mc.LogTipUpdated("System", lastTip, tip.Hash, tip.Height)
					lastTip = tip.Hash
				}
				if tip.Height >= target {
					return
				}
			}
		}
	}
}

// spawnMiner creates, registers, and starts a miner. Returns its cancel function.
func spawnMiner(
	id string,
	miningDelay time.Duration,
	chain *core.BlockChain,
	net *network.Network,
	outbound chan<- *core.Block,
	mc *MetricsCollector,
	parentCtx context.Context,
) context.CancelFunc {
	mCtx, mCancel := context.WithCancel(parentCtx)
	inbound := make(chan *core.Block, 100)
	net.Register(id, inbound)

	m := miner.NewMiner(id, miningDelay, chain, inbound, outbound)

	// Wire up telemetry callbacks.
	m.OnTipUpdated = func(minerID, oldTip, newTip string, height uint64) {
		mc.LogTipUpdated(minerID, oldTip, newTip, height)
	}
	m.OnBlockMined = func(minerID, hash string, difficulty float64, height uint64) {
		// Block mining is already logged by the router.
		// This callback can be used for miner-side telemetry if needed.
	}

	mc.LogMinerStatus(id, "joined")
	m.Start(mCtx)
	fmt.Printf("  [%s] Miner joined (delay=%v)\n", id, miningDelay)
	return mCancel
}

// printMainChain walks the heaviest chain and prints block details.
func printMainChain(chain *core.BlockChain) {
	tip := chain.GetHeaviestChainTip()
	var history []*core.BlockNode
	for tip != nil {
		history = append([]*core.BlockNode{tip}, history...)
		tip = tip.Parent
	}

	fmt.Println("\n  Block | Difficulty  | Production Time (ms)")
	fmt.Println("  ------------------------------------------")
	for i := 1; i < len(history); i++ {
		current := history[i]
		previous := history[i-1]
		prodTime := current.Block.Header.Timestamp - previous.Block.Header.Timestamp
		fmt.Printf("  %5d | %11.4f | %14d\n", current.Height, current.Block.Header.Difficulty, prodTime)
	}
}

// ---------------------------------------------------------------------------
// Experiment A: Baseline (1 miner, 30 blocks)
// ---------------------------------------------------------------------------

func RunExperimentA() {
	fmt.Println("\n=== Experiment A: Baseline (1 Miner, Target: 30 Blocks) ===")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_a.jsonl")
	defer mc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	cancelM1 := spawnMiner("M1", 5*time.Millisecond, chain, net, outbound, mc, ctx)
	_ = cancelM1

	waitForHeight(ctx, chain, 30, mc)
	cancel()
	wg.Wait()

	printMainChain(chain)
	fmt.Println("  Experiment A complete.")
}

// ---------------------------------------------------------------------------
// Experiment B: Hashrate Shock (dynamic miners, observe difficulty)
// ---------------------------------------------------------------------------

func RunExperimentB() {
	fmt.Println("\n=== Experiment B: Hashrate Shock (Dynamic Miners & Difficulty) ===")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_b.jsonl")
	defer mc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	miners := make(map[string]context.CancelFunc)
	delay := 5 * time.Millisecond

	addMiner := func(id string) {
		miners[id] = spawnMiner(id, delay, chain, net, outbound, mc, ctx)
	}

	removeMiner := func(id string) {
		if c, exists := miners[id]; exists {
			c()
			net.Unregister(id)
			mc.LogMinerStatus(id, "left")
			fmt.Printf("  [%s] Miner left.\n", id)
			delete(miners, id)
		}
	}

	// Phase 1: Start with 1 miner.
	addMiner("M1")
	time.Sleep(5 * time.Second)

	// Phase 2: Add M2 (total: 2 miners).
	addMiner("M2")
	time.Sleep(5 * time.Second)

	// Phase 3: Add M3 (total: 3 miners).
	addMiner("M3")
	time.Sleep(5 * time.Second)

	// Phase 4: Remove M3 (total: 2 miners).
	removeMiner("M3")
	time.Sleep(5 * time.Second)

	// Phase 5: Remove M2 (total: 1 miner).
	removeMiner("M2")
	time.Sleep(10 * time.Second)

	cancel()
	wg.Wait()

	printMainChain(chain)
	fmt.Println("  Experiment B complete.")
}

// ---------------------------------------------------------------------------
// Experiment C: Hashrate Ratio (2 miners, 1:3 speed ratio, Δ=0)
// ---------------------------------------------------------------------------

func RunExperimentC() {
	fmt.Println("\n=== Experiment C: Hashrate Distribution (2 Miners, 1:3, Δ=0) ===")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_c.jsonl")
	defer mc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	// M1: 10µs delay (fast), M2: 30µs delay (slow) → M1 is ~3x faster.
	_ = spawnMiner("M1", 10*time.Microsecond, chain, net, outbound, mc, ctx)
	_ = spawnMiner("M2", 30*time.Microsecond, chain, net, outbound, mc, ctx)

	waitForHeight(ctx, chain, 40, mc)
	cancel()
	wg.Wait()

	// Count main-chain blocks per miner.
	m1Count, m2Count := 0, 0
	tip := chain.GetHeaviestChainTip()
	for tip != nil {
		if strings.Contains(tip.Block.Body, "M1") {
			m1Count++
		} else if strings.Contains(tip.Block.Body, "M2") {
			m2Count++
		}
		tip = tip.Parent
	}

	total := m1Count + m2Count
	if total > 0 {
		fmt.Printf("  Main Chain Blocks: Total=%d | M1=%.1f%% (%d) | M2=%.1f%% (%d)\n",
			total, float64(m1Count)/float64(total)*100, m1Count,
			float64(m2Count)/float64(total)*100, m2Count)
	}
	fmt.Println("  Experiment C complete.")
}

// ---------------------------------------------------------------------------
// Experiment D: Fork Analysis (2 miners, Δ ∈ {0.5, 1, 2}s)
// ---------------------------------------------------------------------------

func RunExperimentD() {
	deltas := []struct {
		d    time.Duration
		name string
	}{
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1s"},
		{2 * time.Second, "2s"},
	}

	for _, entry := range deltas {
		fmt.Printf("\n=== Experiment D: Fork Analysis (Δ=%s) ===\n", entry.name)

		logName := fmt.Sprintf("metrics_exp_d_%s.jsonl", entry.name)
		chain, net, outbound, mc := setupEnvironment(entry.d, logName)

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		startRouter(ctx, &wg, net, outbound, mc)

		// Observer channel to track ALL mined blocks (including those that become orphans).
		observerChan := make(chan *core.Block, 2000)
		net.Register("Observer", observerChan)

		var observerMu sync.Mutex
		allMinedBlocks := make([]*core.Block, 0, 200)

		var observerWg sync.WaitGroup
		observerWg.Add(1)
		go func() {
			defer observerWg.Done()
			for {
				select {
				case b := <-observerChan:
					observerMu.Lock()
					allMinedBlocks = append(allMinedBlocks, b)
					observerMu.Unlock()
				case <-ctx.Done():
					// Drain remaining buffered blocks after cancellation.
					for {
						select {
						case b := <-observerChan:
							observerMu.Lock()
							allMinedBlocks = append(allMinedBlocks, b)
							observerMu.Unlock()
						default:
							return
						}
					}
				}
			}
		}()

		minerCtx, cancelMiners := context.WithCancel(ctx)

		// M1: fast (10µs), M2: slow (30µs) → 1:3 ratio.
		_ = spawnMiner("M1", 10*time.Microsecond, chain, net, outbound, mc, minerCtx)
		_ = spawnMiner("M2", 30*time.Microsecond, chain, net, outbound, mc, minerCtx)

		startTime := time.Now()

		waitForHeight(ctx, chain, 40, mc)
		elapsed := time.Since(startTime)

		// Stop miners first to prevent new blocks from being found.
		cancelMiners()

		// Allow in-flight block propagation to complete.
		time.Sleep(entry.d*2 + 50*time.Millisecond)

		// Stop routing and observing.
		cancel()
		wg.Wait()
		observerWg.Wait()
		mc.Close()

		// Build the set of main-chain block hashes.
		mainChain := make(map[string]bool)
		tip := chain.GetHeaviestChainTip()
		for tip != nil {
			mainChain[tip.Hash] = true
			tip = tip.Parent
		}

		// Count orphans per miner and fork points.
		orphansM1, orphansM2 := 0, 0
		childrenCount := make(map[string]int)

		observerMu.Lock()
		for _, b := range allMinedBlocks {
			childrenCount[b.Header.Parent]++

			if !mainChain[b.Hash()] {
				if strings.Contains(b.Body, "M1") {
					orphansM1++
				} else if strings.Contains(b.Body, "M2") {
					orphansM2++
				}
			}
		}
		observerMu.Unlock()

		totalForks := 0
		for _, count := range childrenCount {
			if count > 1 {
				totalForks += (count - 1)
			}
		}

		fmt.Printf("  Elapsed Time to 40 Blocks : %v\n", elapsed)
		fmt.Printf("  Total Forks Occurred      : %d\n", totalForks)
		fmt.Printf("  Orphan Blocks             : M1=%d, M2=%d (Total: %d)\n", orphansM1, orphansM2, orphansM1+orphansM2)
	}
	fmt.Println("  Experiment D complete.")
}
