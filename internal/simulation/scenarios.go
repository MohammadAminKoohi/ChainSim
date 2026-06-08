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

func setupEnvironment(delta time.Duration) (*core.BlockChain, *network.Network, chan *core.Block) {
	genesis := &core.Block{
		Header: core.BlockHeader{Difficulty: 1.0, Timestamp: time.Now().UnixMilli()},
		Body:   "Genesis",
	}
	chain, _ := core.NewBlockChain(genesis)
	net := network.NewNetwork(delta)
	outbound := make(chan *core.Block, 1000)
	return chain, net, outbound
}

func startRouter(ctx context.Context, wg *sync.WaitGroup, net *network.Network, outbound <-chan *core.Block) {
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
				if len(parts) >= 3 && parts[0] == "Mined" {
					senderID = parts[2]
				}
				net.Broadcast(b, senderID)
			}
		}
	}()
}

func waitForHeight(ctx context.Context, chain *core.BlockChain, target uint64) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if tip := chain.GetHeaviestChainTip(); tip != nil && tip.Height >= target {
				return
			}
		}
	}
}

func RunExperimentA() {
	fmt.Println("\n--- Starting Experiment A (1 Miner, Target: 30 Blocks) ---")
	chain, net, outbound := setupEnvironment(0)
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound)

	inbound := make(chan *core.Block, 100)
	net.Register("M1", inbound)
	m1 := miner.NewMiner("M1", 1.0, 5*time.Millisecond, chain, inbound, outbound)
	
	m1.Start(ctx) 

	waitForHeight(ctx, chain, 30)
	cancel()
	wg.Wait()

	fmt.Printf("Experiment A Completed. Final Chain Height: %d\n", chain.GetHeaviestChainTip().Height)
}

func RunExperimentB() {
	fmt.Println("\n--- Starting Experiment B (Dynamic Miners & Difficulty) ---")
	chain, net, outbound := setupEnvironment(0)
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound)

	miners := make(map[string]context.CancelFunc)
	
	spawnMiner := func(id string) {
		mCtx, mCancel := context.WithCancel(ctx)
		miners[id] = mCancel
		inbound := make(chan *core.Block, 100)
		net.Register(id, inbound)
		m := miner.NewMiner(id, 1.0, 5*time.Millisecond, chain, inbound, outbound)
		m.Start(mCtx)
		fmt.Printf("[%s] Miner joined.\n", id)
	}

	killMiner := func(id string) {
		if c, exists := miners[id]; exists {
			c() 
			net.Unregister(id)
			fmt.Printf("[%s] Miner left.\n", id)
		}
	}

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

	fmt.Println("Difficulty History (Tip to Genesis):")
	tip := chain.GetHeaviestChainTip()
	for tip != nil {
		if tip.Height%core.DifficultyEpoch == 0 {
			fmt.Printf("Height %d: %.2f\n", tip.Height, tip.Block.Header.Difficulty)
		}
		tip = tip.Parent
	}
}

func RunExperimentC() {
	fmt.Println("\n--- Starting Experiment C (Hashrate Distribution, Delta: 0) ---")
	chain, net, outbound := setupEnvironment(0)
	
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	startRouter(ctx, &wg, net, outbound)

	baseDelay := 10 * time.Millisecond

	in1, in2 := make(chan *core.Block, 100), make(chan *core.Block, 100)
	net.Register("M1", in1)
	net.Register("M2", in2)

	m1 := miner.NewMiner("M1", 3.0, baseDelay, chain, in1, outbound)
	m2 := miner.NewMiner("M2", 1.0, baseDelay, chain, in2, outbound)

	m1.Start(ctx)
	m2.Start(ctx)

	waitForHeight(ctx, chain, 40)
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
		chain, net, outbound := setupEnvironment(delta)
		
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		startRouter(ctx, &wg, net, outbound)

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

		// Speed adjusted so blocks aren't completely drowned by the high latency
		m1 := miner.NewMiner("M1", 3.0, 50*time.Millisecond, chain, in1, outbound)
		m2 := miner.NewMiner("M2", 1.0, 50*time.Millisecond, chain, in2, outbound)

		m1.Start(ctx)
		m2.Start(ctx)

		waitForHeight(ctx, chain, 40)
		cancel() 
		wg.Wait()
		observerWg.Wait()

		mainChain := make(map[string]bool)
		tip := chain.GetHeaviestChainTip()
		for tip != nil {
			mainChain[tip.Hash] = true
			tip = tip.Parent
		}

		orphansM1, orphansM2 := 0, 0
		for _, b := range allMinedBlocks {
			if !mainChain[b.Hash()] {
				if strings.Contains(b.Body, "M1") {
					orphansM1++
				} else if strings.Contains(b.Body, "M2") {
					orphansM2++
				}
			}
		}

		fmt.Printf("Results for Delta %v:\n", delta)
		fmt.Printf("Total Main Chain Blocks: %d\n", len(mainChain))
		fmt.Printf("Orphan/Fork Blocks: M1=%d, M2=%d (Total Forks: %d)\n", orphansM1, orphansM2, orphansM1+orphansM2)
	}
}