package simulation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"strconv"

	"github.com/MohammadAminKoohi/ChainSim/internal/core"
	"github.com/MohammadAminKoohi/ChainSim/internal/miner"
	"github.com/MohammadAminKoohi/ChainSim/internal/network"
)

func setupEnvironment(delta time.Duration, logFilename string) (*core.BlockChain, *network.Network, chan *core.Block, *MetricsCollector) {
	genesis := &core.Block{
		Header: core.BlockHeader{Difficulty: 1.0, Timestamp: time.Now().UnixMilli()},
		Body:   "Genesis",
	}
	chain, _ := core.NewBlockChain(genesis)
	net := network.NewNetwork(delta)
	outbound := make(chan *core.Block, 1000)
	
	mc, err := NewMetricsCollector(logFilename)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize metrics collector for %s: %s", logFilename, err.Error()))
	}

	return chain, net, outbound, mc
}

func startRouter(ctx context.Context, wg *sync.WaitGroup, net *network.Network, outbound <-chan *core.Block, mc *MetricsCollector) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-outbound:
				parts := strings.Split(b.Body, " ")
				senderID := "unknown"
				var height uint64 = 0

				// The miner formats the body as: "Mined by [ID] at height [height]"
				// Example: "Mined by M1 at height 5" (Length is 6)
				if len(parts) >= 6 && parts[0] == "Mined" {
					senderID = parts[2]
					// Extract and parse the height from the end of the string
					if parsedHeight, err := strconv.ParseUint(parts[5], 10, 64); err == nil {
						height = parsedHeight
					}
				}
				
				// Now we pass the actual height instead of a hardcoded 0
				mc.LogBlockMined(senderID, b.Hash(), b.Header.Difficulty, height)
				
				net.Broadcast(b, senderID)
			}
		}
	}()
}

func waitForHeight(ctx context.Context, chain *core.BlockChain, target uint64, mc *MetricsCollector) {
	ticker := time.NewTicker(100 * time.Millisecond)
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

func RunExperimentA() {
	fmt.Println("\n--- Starting Experiment A (1 Miner, Target: 30 Blocks) ---")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_a.jsonl")
	defer mc.Close()
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	inbound := make(chan *core.Block, 100)
	net.Register("M1", inbound)
	m1 := miner.NewMiner("M1", 1.0, 5*time.Millisecond, chain, inbound, outbound)
	
	mc.LogMinerStatus("M1", "joined")
	m1.Start(ctx)

	waitForHeight(ctx, chain, 30, mc)
	cancel()
	wg.Wait()

	tip := chain.GetHeaviestChainTip()
	var history []*core.BlockNode
	for tip != nil {
		history = append([]*core.BlockNode{tip}, history...) 
		tip = tip.Parent
	}

	fmt.Println("\nBlock | Difficulty | Production Time (ms)")
	fmt.Println("-----------------------------------------")
	for i := 1; i < len(history); i++ {
		current := history[i]
		previous := history[i-1]
		prodTime := current.Block.Header.Timestamp - previous.Block.Header.Timestamp
		fmt.Printf("%5d | %10.2f | %14d\n", current.Height, current.Block.Header.Difficulty, prodTime)
	}
}

func RunExperimentB() {
	fmt.Println("\n--- Starting Experiment B (Dynamic Miners & Difficulty) ---")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_b.jsonl")
	defer mc.Close()
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	miners := make(map[string]context.CancelFunc)
	
	spawnMiner := func(id string) {
		mCtx, mCancel := context.WithCancel(ctx)
		miners[id] = mCancel
		inbound := make(chan *core.Block, 100)
		net.Register(id, inbound)
		m := miner.NewMiner(id, 1.0, 5*time.Millisecond, chain, inbound, outbound)
		
		mc.LogMinerStatus(id, "joined")
		m.Start(mCtx)
		fmt.Printf("[%s] Miner joined.\n", id)
	}

	killMiner := func(id string) {
		if c, exists := miners[id]; exists {
			c() 
			net.Unregister(id)
			mc.LogMinerStatus(id, "left")
			fmt.Printf("[%s] Miner left.\n", id)
		}
	}

	go func() {
		waitForHeight(ctx, chain, 100, mc) 
	}()

	spawnMiner("M1")
	time.Sleep(5 * time.Second)
	spawnMiner("M2")
	time.Sleep(5 * time.Second)
	spawnMiner("M3")
	time.Sleep(5 * time.Second)

	killMiner("M3")
	time.Sleep(5 * time.Second)
	killMiner("M2")
	time.Sleep(5 * time.Second)

	cancel() 
	wg.Wait()
	fmt.Println("Experiment B Sequence Complete.")
}

func RunExperimentC() {
	fmt.Println("\n--- Starting Experiment C (Hashrate Distribution, Delta: 0) ---")
	chain, net, outbound, mc := setupEnvironment(0, "metrics_exp_c.jsonl")
	defer mc.Close()
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound, mc)

	baseDelay := 10 * time.Millisecond

	in1, in2 := make(chan *core.Block, 100), make(chan *core.Block, 100)
	net.Register("M1", in1)
	net.Register("M2", in2)

	m1 := miner.NewMiner("M1", 3.0, baseDelay, chain, in1, outbound)
	m2 := miner.NewMiner("M2", 1.0, baseDelay, chain, in2, outbound)

	mc.LogMinerStatus("M1", "joined")
	mc.LogMinerStatus("M2", "joined")

	m1.Start(ctx)
	m2.Start(ctx)

	waitForHeight(ctx, chain, 40, mc)
	cancel()
	wg.Wait()

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
	fmt.Printf("Main Chain Blocks: Total=%d | M1=%.2f%% | M2=%.2f%%\n", 
		total, float64(m1Count)/float64(total)*100, float64(m2Count)/float64(total)*100)
}

func RunExperimentD() {
	deltas := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
	}

	for _, delta := range deltas {
		fmt.Printf("\n--- Starting Experiment D (Delta: %v) ---\n", delta)
		
		logName := fmt.Sprintf("metrics_exp_d_%v.jsonl", delta)
		chain, net, outbound, mc := setupEnvironment(delta, logName)
		
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		startRouter(ctx, &wg, net, outbound, mc)

		observerChan := make(chan *core.Block, 1000)
		net.Register("Observer", observerChan)
		
		var observerWg sync.WaitGroup
		observerWg.Add(1)
		allMinedBlocks := make([]*core.Block, 0)
		
		go func() {
			defer observerWg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-observerChan:
					allMinedBlocks = append(allMinedBlocks, b)
				}
			}
		}()

		in1, in2 := make(chan *core.Block, 100), make(chan *core.Block, 100)
		net.Register("M1", in1)
		net.Register("M2", in2)

		m1 := miner.NewMiner("M1", 3.0, 50*time.Millisecond, chain, in1, outbound)
		m2 := miner.NewMiner("M2", 1.0, 50*time.Millisecond, chain, in2, outbound)

		startTime := time.Now()

		m1.Start(ctx)
		m2.Start(ctx)

		waitForHeight(ctx, chain, 40, mc)
		
		elapsedTime := time.Since(startTime)
		
		cancel() 
		wg.Wait()
		observerWg.Wait() 
		mc.Close()

		mainChain := make(map[string]bool)
		tip := chain.GetHeaviestChainTip()
		for tip != nil {
			mainChain[tip.Hash] = true
			tip = tip.Parent
		}

		orphansM1, orphansM2 := 0, 0
		childrenCount := make(map[string]int)

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

		totalForks := 0
		for _, count := range childrenCount {
			if count > 1 {
				totalForks += (count - 1)
			}
		}

		fmt.Printf("Results for Delta %v:\n", delta)
		fmt.Printf("  Elapsed Time to 40 Blocks : %v\n", elapsedTime)
		fmt.Printf("  Total Forks Occurred      : %d\n", totalForks)
		fmt.Printf("  Discarded Blocks          : M1=%d, M2=%d (Total: %d)\n", orphansM1, orphansM2, orphansM1+orphansM2)
	}
}