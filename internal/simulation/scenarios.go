package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		Header: core.BlockHeader{Difficulty: 100.0, Timestamp: 1719878400000}, // Fixed timestamp and realistic starting difficulty
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

// spawnMiner creates, registers, and starts a miner. Returns its miner instance and cancel function.
func spawnMiner(
	id string,
	miningDelay time.Duration,
	chain *core.BlockChain,
	net *network.Network,
	outbound chan<- *core.Block,
	mc *MetricsCollector,
	parentCtx context.Context,
) (*miner.Miner, context.CancelFunc) {
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
	return m, mCancel
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

	m1, cancelM1 := spawnMiner("M1", 5*time.Millisecond, chain, net, outbound, mc, ctx)
	_ = m1
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

	type minerInfo struct {
		m      *miner.Miner
		cancel context.CancelFunc
	}
	miners := make(map[string]minerInfo)
	delay := 5 * time.Millisecond

	addMiner := func(id string) {
		var mChain *core.BlockChain
		if id == "M1" {
			mChain = chain
		} else {
			if m1Info, exists := miners["M1"]; exists {
				mChain = m1Info.m.Chain.Copy()
			} else {
				mChain = chain.Copy()
			}
		}
		m, mCancel := spawnMiner(id, delay, mChain, net, outbound, mc, ctx)
		miners[id] = minerInfo{m, mCancel}
	}

	removeMiner := func(id string) {
		if info, exists := miners[id]; exists {
			info.m.StopMining()
			info.cancel()
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
	// They mine on independent chains, starting from the same genesis state.
	m1, cancelM1 := spawnMiner("M1", 10*time.Microsecond, chain, net, outbound, mc, ctx)
	m2, cancelM2 := spawnMiner("M2", 30*time.Microsecond, chain.Copy(), net, outbound, mc, ctx)
	_ = cancelM1
	_ = cancelM2

	waitForHeight(ctx, chain, 40, mc)
	m1.StopMining()
	m2.StopMining()

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
		// They mine on independent chains, starting from the same genesis state.
		m1, _ := spawnMiner("M1", 10*time.Microsecond, chain, net, outbound, mc, minerCtx)
		m2, _ := spawnMiner("M2", 30*time.Microsecond, chain.Copy(), net, outbound, mc, minerCtx)

		startTime := time.Now()

		waitForHeight(ctx, chain, 40, mc)
		elapsed := time.Since(startTime)

		// Stop mining on both miners.
		m1.StopMining()
		m2.StopMining()

		// Allow in-flight block propagation to complete.
		time.Sleep(entry.d*2 + 50*time.Millisecond)

		// Stop routing and observing.
		cancelMiners()
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

// SelfishResult holds the metrics for a single simulation run.
type SelfishResult struct {
	R                  int     `json:"r"`
	AvgBlockTimeMs     float64 `json:"avg_block_time_ms"`
	AttackerShare      float64 `json:"attacker_share"`
	AttackerRevenue    float64 `json:"attacker_revenue"`
	ChainQuality       float64 `json:"chain_quality"`
	Forks              int     `json:"forks"`
	DiscardedBlocks    int     `json:"discarded_blocks"`
	WasteRate          float64 `json:"waste_rate"`
}

func RunSelfishMiningExperiment() {
	fmt.Println("\n=== Selfish Mining Experiment (α = 35%) ===")
	
	RValues := []int{3, 6, 12, 24, 48}
	var results []SelfishResult

	for _, R := range RValues {
		fmt.Printf("\nRunning simulation for R = %d...\n", R)
		
		// 1. Setup Environment
		genesis := &core.Block{
			Header: core.BlockHeader{Difficulty: 100.0, Timestamp: 1719878400000},
			Body:   "Genesis",
		}
		chain, _ := core.NewBlockChain(genesis)
		chain.EpochLength = uint64(R)
		chain.Gamma = 0.5 // Eyal-Sirer tie breaking parameter
		chain.TargetTimePerBlock = 10 // 10ms target block time for fast simulation!

		net := network.NewNetwork(0) // Delta = 0 for instant network propagation (Markov model)
		outbound := make(chan *core.Block, 2000)

		// Create a mock metrics collector that doesn't print, to avoid huge logs
		logName := fmt.Sprintf("metrics_selfish_R_%d.jsonl", R)
		mc, _ := NewMetricsCollector(resultsDir, logName)

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		startRouter(ctx, &wg, net, outbound, mc)

		// Spawning miners
		// M1 (honest): 65% hashrate -> delay 3.5µs
		// M2 (attacker): 35% hashrate -> delay 6.5µs, IsSelfish = true
		m1Ctx, cancelM1 := context.WithCancel(ctx)
		m2Ctx, cancelM2 := context.WithCancel(ctx)

		m1, _ := spawnMiner("M1", 3500*time.Nanosecond, chain, net, outbound, mc, m1Ctx)
		m2, _ := spawnMiner("M2", 6500*time.Nanosecond, chain.Copy(), net, outbound, mc, m2Ctx)
		m2.IsSelfish = true

		// Wait until M1's chain (reference node) reaches 500 blocks.
		waitForHeight(ctx, chain, 500, mc)

		// Stop mining
		m1.StopMining()
		m2.StopMining()

		// Sleep briefly to drain any pending blocks
		time.Sleep(100 * time.Millisecond)

		// Clean up goroutines
		cancelM1()
		cancelM2()
		cancel()
		wg.Wait()
		mc.Close()

		// 2. Calculate Metrics
		tip := chain.GetHeaviestChainTip()
		var history []*core.BlockNode
		for tip != nil {
			history = append([]*core.BlockNode{tip}, history...)
			tip = tip.Parent
		}

		// Count attacker blocks on the main chain
		attackerBlocksOnMain := 0
		for _, node := range history {
			if strings.Contains(node.Block.Body, "M2") {
				attackerBlocksOnMain++
			}
		}

		totalMainChainBlocks := len(history) - 1 // Exclude genesis block
		if totalMainChainBlocks < 1 {
			totalMainChainBlocks = 1
		}

		attackerShare := float64(attackerBlocksOnMain) / float64(totalMainChainBlocks)
		chainQuality := 1.0 - attackerShare

		// Count forks (number of nodes in the chain tree with >1 children)
		forks := 0
		allNodes := chain.GetAllNodes()
		childrenCount := make(map[string]int)
		for _, node := range allNodes {
			if node.Parent != nil {
				childrenCount[node.Parent.Hash]++
			}
		}
		for _, count := range childrenCount {
			if count > 1 {
				forks += (count - 1)
			}
		}

		// Discarded blocks and Waste rate
		totalMinedBlocks := m1.MinedCount + m2.MinedCount
		discardedBlocks := int(totalMinedBlocks) - totalMainChainBlocks
		if discardedBlocks < 0 {
			discardedBlocks = 0
		}
		wasteRate := float64(discardedBlocks) / float64(totalMinedBlocks)

		// Average block generation time
		genesisTimestamp := history[0].Block.Header.Timestamp
		tipTimestamp := history[len(history)-1].Block.Header.Timestamp
		durationMs := float64(tipTimestamp - genesisTimestamp)
		avgBlockTimeMs := durationMs / float64(totalMainChainBlocks)

		res := SelfishResult{
			R:                  R,
			AvgBlockTimeMs:     avgBlockTimeMs,
			AttackerShare:      attackerShare,
			AttackerRevenue:    attackerShare, // Relative revenue is the same
			ChainQuality:       chainQuality,
			Forks:              forks,
			DiscardedBlocks:    discardedBlocks,
			WasteRate:          wasteRate,
		}
		results = append(results, res)

		fmt.Printf("Results for R = %d:\n", R)
		fmt.Printf("  Avg Block Time:   %.2f ms\n", avgBlockTimeMs)
		fmt.Printf("  Attacker Share:   %.2f%%\n", attackerShare*100)
		fmt.Printf("  Attacker Revenue: %.2f%%\n", attackerShare*100)
		fmt.Printf("  Chain Quality:    %.2f%%\n", chainQuality*100)
		fmt.Printf("  Forks:            %d\n", forks)
		fmt.Printf("  Discarded Blocks: %d (out of %d total mined)\n", discardedBlocks, totalMinedBlocks)
		fmt.Printf("  Waste Rate:       %.2f%%\n", wasteRate*100)
	}

	// Write results to results/selfish_results.json
	resBytes, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(filepath.Join(resultsDir, "selfish_results.json"), resBytes, 0644)
	fmt.Println("\nSelfish mining results saved to results/selfish_results.json")
}
