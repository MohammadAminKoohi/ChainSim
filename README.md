# ChainSim — Bitcoin Blockchain Simulator

ChainSim is a concurrent, highly deterministic blockchain simulator written in Go. It models a proof-of-work (PoW) Nakamoto consensus blockchain, including mining nodes, network block propagation latencies, dynamic difficulty adjustments, fork resolution, and hashrate shocks.

The simulator generates structured JSONL telemetry files under the `results/` directory, which can be visualized using a Python plotting script to produce publication-quality charts.

---

## Architecture & Core Design

ChainSim is structured around the following components:
1. **Core Consensus (`internal/core`):**
   - Implements `Block` and `BlockHeader` structures.
   - Validates Proof of Work against a dynamic target: $\text{Target} = \frac{2^{256} - 1}{\text{Difficulty}}$.
   - Implements `BlockChain` as a thread-safe tree of `BlockNode`s, maintaining Nakamoto consensus (where the heaviest chain with the highest cumulative difficulty wins).
   - Adjusts difficulty dynamically using a sliding window of past block intervals on epoch boundaries.
2. **Miner Node (`internal/miner`):**
   - Runs as an independent concurrent goroutine mining on top of the heaviest known chain tip.
   - **Hash Batching:** Solves the OS `time.Sleep` limitation for sub-millisecond delays. By batching hashes together, miners can accurately simulate high hashrates (e.g. 10µs/hash) and maintain exact relative mining speed ratios (e.g., 1:3 hashrate distribution) without burning excessive CPU or succumbing to OS scheduling jitter.
   - **Deterministic Clock:** Blocks calculate simulated timestamps based on `parentTimestamp + (Nonce * MiningDelay)`. This decouples simulation time from the host's execution speed, guaranteeing that block production times and difficulty adjustments are 100% reproducible across different machines.
3. **Network Broker (`internal/network`):**
   - A centralized broker that broadcasts mined blocks to all registered peers.
   - Simulates block propagation latency ($\Delta$) using non-blocking channels and configurable delivery delays.
4. **Simulation Engine (`internal/simulation`):**
   - Coordinates the setup of miners and network latency, registers telemetries, and executes the simulation scenarios.

---

## Directory Structure

```
├── cmd/
│   └── simulator/
│       └── main.go           # Simulator entrypoint running all experiments
├── internal/
│   ├── core/
│   │   ├── block.go          # Block structures, hashing, and PoW validation
│   │   ├── chain.go          # Nakamoto chain tree, fork selection, and difficulty adjustment
│   │   └── chain_test.go     # Unit tests verifying difficulty adjustment math and consensus rules
│   ├── miner/
│   │   └── miner.go          # PoW miner goroutine loop with hash batching and simulated clock
│   ├── network/
│   │   └── network.go        # Network broker simulating propagation delays (Delta)
│   └── simulation/
│       ├── metrics.go        # JSONL telemetry logging utility
│       └── scenarios.go      # Scenario specifications for Experiments A, B, C, and D
├── results/                  # Directory where JSONL telemetry and PNG plots are saved
├── scripts/
│   ├── .venv/                # Python virtual environment (if initialized)
│   └── plot_metrics.py       # Matplotlib script to visualize logs
├── go.mod                    # Go module config
└── README.md                 # Project documentation (this file)
```

---

## Getting Started

### Prerequisites
- **Go:** Version 1.24 or later.
- **Python 3:** (Optional, for generating plots) along with `pandas` and `matplotlib`.

---

## Running the Simulator

To run all four experiments and output the simulation logs:
```bash
go run ./cmd/simulator/main.go
```

### Supported Experiments

* **Experiment A: Baseline (1 Miner, Target: 30 Blocks)**
  - Configures a single miner M1 (5ms delay) with no network latency ($\Delta=0$).
  - Observes how the difficulty adjustments converge block production times toward the target of 1s/block.
* **Experiment B: Hashrate Shock (Dynamic Miners & Difficulty)**
  - Dynamically adds and removes miners over a 35-second period.
  - Demonstrates how the blockchain's difficulty automatically rises when hashrate surges (miners join) and decreases when hashrate collapses (miners leave) to maintain stable block intervals.
* **Experiment C: Hashrate Distribution (2 Miners, 1:3 Speed Ratio, $\Delta=0$)**
  - Spawns two competing miners: M1 (fast, 10µs delay) and M2 (slow, 30µs delay).
  - Demonstrates that block production is proportional to hashrate (M1 mines roughly 75% of main chain blocks, M2 mines roughly 25%).
* **Experiment D: Fork Analysis (2 Miners, compete under $\Delta \in \{500\text{ms}, 1\text{s}, 2\text{s}\}$)**
  - Competes M1 (10µs) and M2 (30µs) under different propagation delays.
  - Measures how higher propagation delay ($\Delta$) increases the frequency of blockchain forks and orphan blocks, demonstrating the latency-security tradeoff in PoW blockchains.

---

## Running Unit Tests

ChainSim includes a comprehensive test suite covering block validation, chain reorganization, difficulty adjustment clamping, and boundary/genesis handling.

To run the unit tests:
```bash
go test -v ./internal/...
```

---

## Visualizing Results

To generate plots of the block generation times and network difficulty changes:

1. Navigate to the `scripts/` directory:
   ```bash
   cd scripts
   ```
2. Activate the pre-configured Python virtual environment (if available on your system):
   ```bash
   source .venv/bin/activate
   ```
   *If `.venv` is not configured, install the dependencies manually:*
   ```bash
   pip install pandas matplotlib
   ```
3. Run the visualization script:
   ```bash
   python plot_metrics.py
   ```

The script will read telemetry files from the `results/` directory and output the following charts:
- `exp_a_generation_time.png`: Shows block times for the first 30 blocks in Experiment A relative to the 1s target.
- `exp_b_difficulty.png`: Shows network difficulty adjustments over time in Experiment B with vertical markers indicating when miners joined/left.
- `exp_b_generation_time.png`: Raw block generation intervals alongside the 5-block moving average in Experiment B.
- `exp_a_vs_b_block_time.png`: Overlay comparing average block generation times under stable hashrate (Exp A) versus hashrate shocks (Exp B).
