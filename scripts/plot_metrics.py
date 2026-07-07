"""
ChainSim Visualization Script
Parses JSONL telemetry logs and generates publication-quality plots.

Usage:
    cd scripts/
    python plot_metrics.py
"""

import json
import os
import sys

import pandas as pd
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

# The directory where the Go simulator outputs .jsonl files.
OUTPUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'results')

# Plot style configuration.
plt.rcParams.update({
    'figure.facecolor': 'white',
    'axes.facecolor': '#fafafa',
    'axes.grid': True,
    'grid.alpha': 0.3,
    'grid.linestyle': ':',
    'font.size': 11,
})


def load_data(filepath):
    """Loads JSON Lines into a structured pandas DataFrame."""
    if not os.path.exists(filepath):
        print(f"  ⚠ Skipping {os.path.basename(filepath)} — file not found.")
        return pd.DataFrame()

    data = []
    with open(filepath, 'r') as f:
        for line in f:
            stripped = line.strip()
            if stripped:
                data.append(json.loads(stripped))

    if not data:
        print(f"  ⚠ Skipping {os.path.basename(filepath)} — file is empty.")
        return pd.DataFrame()

    return pd.json_normalize(data)


# ---------------------------------------------------------------------------
# Plot 1: Experiment B — Network Difficulty over Time
# ---------------------------------------------------------------------------

def plot_exp_b_difficulty(df_b):
    """Plots Network Difficulty over Simulated Time for Experiment B,
    with vertical markers where miners joined/left."""
    if df_b.empty:
        return

    blocks = df_b[df_b['type'] == 'block_mined'].copy()
    miners = df_b[df_b['type'] == 'miner_status'].copy()

    if blocks.empty:
        print("  ⚠ No block_mined events in Experiment B data.")
        return

    start_time = df_b['timestamp'].min()
    blocks['rel_time'] = (blocks['timestamp'] - start_time) / 1000.0

    fig, ax = plt.subplots(figsize=(13, 6))
    ax.plot(blocks['rel_time'], blocks['data.difficulty'],
            drawstyle='steps-post', color='#1f77b4', linewidth=2.2,
            label='Network Difficulty')

    # Mark miner join/leave events.
    for _, row in miners.iterrows():
        rel_time = max(0, (row['timestamp'] - start_time) / 1000.0)
        status = row.get('data.status', '')
        miner_id = row.get('data.miner_id', '?')

        color = '#2ca02c' if status == 'joined' else '#d62728'
        ax.axvline(x=rel_time, color=color, linestyle='--', alpha=0.75, linewidth=1.2)
        ax.text(rel_time + 0.3, ax.get_ylim()[1] * 0.92,
                f"{miner_id} {status}", rotation=90,
                verticalalignment='top', color=color, fontweight='bold', fontsize=9)

    ax.set_title('Experiment B: Network Difficulty vs. Simulated Time', fontsize=14, fontweight='bold')
    ax.set_xlabel('Simulated Time (seconds)')
    ax.set_ylabel('Difficulty')
    ax.legend(loc='upper left')
    fig.tight_layout()

    out_path = os.path.join(OUTPUT_DIR, 'exp_b_difficulty.png')
    fig.savefig(out_path, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path}")


# ---------------------------------------------------------------------------
# Plot 2: Experiment B — Block Generation Time
# ---------------------------------------------------------------------------

def plot_exp_b_generation_time(df_b):
    """Plots per-block generation time for Experiment B."""
    if df_b.empty:
        return

    blocks = df_b[df_b['type'] == 'block_mined'].copy()
    if len(blocks) < 2:
        return

    blocks = blocks.sort_values('timestamp').reset_index(drop=True)
    blocks['production_time'] = blocks['timestamp'].diff() / 1000.0

    fig, ax = plt.subplots(figsize=(13, 6))
    window = 5

    ax.scatter(blocks['data.height'], blocks['production_time'],
               color='#1f77b4', alpha=0.4, s=20, label='Raw Block Time')
    ax.plot(blocks['data.height'],
            blocks['production_time'].rolling(window=window, min_periods=1).mean(),
            color='#000080', linewidth=2.5, label=f'{window}-Block Moving Avg')
    ax.axhline(y=1.0, color='red', linestyle='--', alpha=0.7, linewidth=1.5,
               label='Target (1.0s)')

    ax.set_title('Experiment B: Block Generation Time', fontsize=14, fontweight='bold')
    ax.set_xlabel('Block Height')
    ax.set_ylabel('Generation Time (seconds)')
    ax.legend(loc='upper right')
    fig.tight_layout()

    out_path = os.path.join(OUTPUT_DIR, 'exp_b_generation_time.png')
    fig.savefig(out_path, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path}")


# ---------------------------------------------------------------------------
# Plot 3: Experiment A — Block Generation Time (Blocks 1–30)
# ---------------------------------------------------------------------------

def plot_exp_a_generation_time(df_a):
    """Plots block generation time for the first 30 blocks in Experiment A."""
    if df_a.empty:
        return

    blocks = df_a[df_a['type'] == 'block_mined'].copy()
    blocks = blocks[blocks['data.height'] <= 30].copy()
    if len(blocks) < 2:
        return

    blocks = blocks.sort_values('data.height').reset_index(drop=True)
    blocks['production_time'] = blocks['timestamp'].diff() / 1000.0

    fig, ax = plt.subplots(figsize=(13, 6))

    ax.plot(blocks['data.height'], blocks['production_time'],
            marker='o', linestyle='-', color='#ff7f0e', linewidth=2,
            markersize=5, label='Block Generation Time')
    ax.axhline(y=1.0, color='black', linestyle='--', alpha=0.7, linewidth=1.5,
               label='Target (1.0s)')

    ax.set_title('Experiment A: Block Generation Time (Blocks 1–30)', fontsize=14, fontweight='bold')
    ax.set_xlabel('Block Height')
    ax.set_ylabel('Generation Time (seconds)')
    ax.xaxis.set_major_locator(ticker.MaxNLocator(integer=True))
    ax.legend(loc='upper right')
    fig.tight_layout()

    out_path = os.path.join(OUTPUT_DIR, 'exp_a_generation_time.png')
    fig.savefig(out_path, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path}")


# ---------------------------------------------------------------------------
# Plot 4: Experiment A vs. B — Average Block Time Comparison
# ---------------------------------------------------------------------------

def plot_a_vs_b_block_time(df_a, df_b):
    """Overlay plot comparing average block time between Experiments A and B."""
    if df_a.empty or df_b.empty:
        print("  ⚠ Skipping A-vs-B comparison — missing data.")
        return

    def compute_rolling_avg(df, label):
        blocks = df[df['type'] == 'block_mined'].copy()
        if len(blocks) < 2:
            return None
        blocks = blocks.sort_values('timestamp').reset_index(drop=True)
        blocks['production_time'] = blocks['timestamp'].diff() / 1000.0
        blocks['rolling_avg'] = blocks['production_time'].rolling(window=5, min_periods=1).mean()
        blocks['label'] = label
        return blocks

    a_blocks = compute_rolling_avg(df_a, 'Exp A (1 miner)')
    b_blocks = compute_rolling_avg(df_b, 'Exp B (dynamic miners)')

    if a_blocks is None or b_blocks is None:
        return

    fig, ax = plt.subplots(figsize=(13, 6))

    ax.plot(a_blocks['data.height'], a_blocks['rolling_avg'],
            color='#ff7f0e', linewidth=2.2, label='Exp A: 1 Miner (5-block avg)')
    ax.plot(b_blocks['data.height'], b_blocks['rolling_avg'],
            color='#1f77b4', linewidth=2.2, label='Exp B: Dynamic Miners (5-block avg)')
    ax.axhline(y=1.0, color='red', linestyle='--', alpha=0.7, linewidth=1.5,
               label='Target (1.0s)')

    ax.set_title('Average Block Time: Experiment A vs. Experiment B', fontsize=14, fontweight='bold')
    ax.set_xlabel('Block Height')
    ax.set_ylabel('Avg Generation Time (seconds)')
    ax.legend(loc='upper right')
    fig.tight_layout()

    out_path = os.path.join(OUTPUT_DIR, 'exp_a_vs_b_block_time.png')
    fig.savefig(out_path, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path}")

# ---------------------------------------------------------------------------
# Plot 5: Selfish Mining Experiment vs R
# ---------------------------------------------------------------------------

def plot_selfish_results():
    results_path = os.path.join(OUTPUT_DIR, 'selfish_results.json')
    if not os.path.exists(results_path):
        print("  ⚠ Skipping Selfish Mining plots — selfish_results.json not found.")
        return

    with open(results_path, 'r') as f:
        results = json.load(f)

    if not results:
        print("  ⚠ Skipping Selfish Mining plots — selfish_results.json is empty.")
        return

    # Extract data
    R = [r['r'] for r in results]
    avg_block_time = [r['avg_block_time_ms'] for r in results]
    attacker_share = [r['attacker_share'] * 100 for r in results]
    chain_quality = [r['chain_quality'] * 100 for r in results]
    forks = [r['forks'] for r in results]
    discarded = [r['discarded_blocks'] for r in results]
    waste_rate = [r['waste_rate'] * 100 for r in results]

    # Print Table to console
    print("\n" + "="*80)
    print("                 SELFISH MINING EXPERIMENT RESULTS (α = 35%)")
    print("="*80)
    print(f"{'R (Epoch)':<12} | {'Avg Time (ms)':<15} | {'Att Share (%)':<15} | {'Chain Quality':<15} | {'Forks':<6} | {'Waste (%)':<10}")
    print("-"*80)
    for r in results:
        print(f"{r['r']:<12} | {r['avg_block_time_ms']:<15.2f} | {r['attacker_share']*100:<15.2f} | {r['chain_quality']*100:<15.2f} | {r['forks']:<6} | {r['waste_rate']*100:<10.2f}")
    print("="*80 + "\n")

    # Generate Plots
    # 1. Attacker Share & Chain Quality vs R
    fig, ax = plt.subplots(figsize=(10, 5.5))
    ax.plot(R, attacker_share, marker='o', color='#d62728', linewidth=2.5, label="Attacker's Share of Main Chain Blocks")
    ax.plot(R, chain_quality, marker='s', color='#2ca02c', linewidth=2.5, label="Chain Quality (Honest Miner Share)")
    ax.axhline(y=35, color='#d62728', linestyle='--', alpha=0.6, label="Attacker Hashrate Share (α = 35%)")
    ax.axhline(y=65, color='#2ca02c', linestyle='--', alpha=0.6, label="Honest Hashrate Share (1 - α = 65%)")
    ax.set_title("Block Distribution & Chain Quality vs. Difficulty Retargeting Interval (R)", fontsize=13, fontweight='bold', pad=15)
    ax.set_xlabel("Epoch Length R (Blocks between difficulty retargets)", labelpad=10)
    ax.set_ylabel("Percentage (%)", labelpad=10)
    ax.set_xticks(R)
    ax.set_ylim(0, 100)
    ax.legend(loc="best", framealpha=0.9)
    fig.tight_layout()
    out_path1 = os.path.join(OUTPUT_DIR, 'selfish_attacker_share.png')
    fig.savefig(out_path1, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path1}")

    # 2. Waste Rate & Orphan Blocks vs R
    fig, ax1 = plt.subplots(figsize=(10, 5.5))
    color = '#1f77b4'
    ax1.set_xlabel("Epoch Length R (Blocks between difficulty retargets)", labelpad=10)
    ax1.set_ylabel("Discarded (Orphan) Blocks Count", color=color, labelpad=10)
    bars = ax1.bar(R, discarded, color=color, alpha=0.4, width=2.0, label="Discarded Blocks")
    ax1.tick_params(axis='y', labelcolor=color)
    ax1.set_xticks(R)
    for bar in bars:
        height = bar.get_height()
        ax1.annotate(f'{int(height)}',
                     xy=(bar.get_x() + bar.get_width() / 2, height),
                     xytext=(0, 3),
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=9, color=color, fontweight='bold')

    ax2 = ax1.twinx()
    color = '#ff7f0e'
    ax2.set_ylabel("Network Hash Power Waste Rate (%)", color=color, labelpad=10)
    ax2.plot(R, waste_rate, color=color, marker='o', linewidth=2.5, label="Waste Rate (%)")
    ax2.tick_params(axis='y', labelcolor=color)
    ax2.set_ylim(0, max(waste_rate) * 1.3 if max(waste_rate) > 0 else 10)
    for x, y in zip(R, waste_rate):
        ax2.annotate(f'{y:.2f}%',
                     xy=(x, y),
                     xytext=(0, 8),
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=9, color=color, fontweight='bold')
    plt.title("Orphan Blocks & Hash Power Waste Rate vs. Retargeting Interval (R)", fontsize=13, fontweight='bold', pad=15)
    fig.tight_layout()
    out_path2 = os.path.join(OUTPUT_DIR, 'selfish_waste_rate.png')
    fig.savefig(out_path2, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path2}")

    # 3. Forks vs R
    fig, ax = plt.subplots(figsize=(10, 5.5))
    ax.plot(R, forks, marker='D', color='#9467bd', linewidth=2.5, label="Total Forks Created")
    for x, y in zip(R, forks):
        ax.annotate(f'{y}',
                     xy=(x, y),
                     xytext=(0, 8),
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=10, fontweight='bold', color='#9467bd')
    ax.set_title("Total Forks Created vs. Difficulty Retargeting Interval (R)", fontsize=13, fontweight='bold', pad=15)
    ax.set_xlabel("Epoch Length R (Blocks between difficulty retargets)", labelpad=10)
    ax.set_ylabel("Forks Count", labelpad=10)
    ax.set_xticks(R)
    ax.set_ylim(0, max(forks) * 1.3 if max(forks) > 0 else 5)
    fig.tight_layout()
    out_path3 = os.path.join(OUTPUT_DIR, 'selfish_forks.png')
    fig.savefig(out_path3, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path3}")

    # 4. Average Block Generation Time vs R
    fig, ax = plt.subplots(figsize=(10, 5.5))
    ax.plot(R, avg_block_time, marker='^', color='#17becf', linewidth=2.5, label="Average Block Production Time")
    for x, y in zip(R, avg_block_time):
        ax.annotate(f'{y:.2f} ms',
                     xy=(x, y),
                     xytext=(0, 8),
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=9, fontweight='bold', color='#17becf')
    ax.set_title("Average Block Production Time vs. Difficulty Retargeting Interval (R)", fontsize=13, fontweight='bold', pad=15)
    ax.set_xlabel("Epoch Length R (Blocks between difficulty retargets)", labelpad=10)
    ax.set_ylabel("Simulated Time (ms)", labelpad=10)
    ax.set_xticks(R)
    ax.set_ylim(0, max(avg_block_time) * 1.3 if max(avg_block_time) > 0 else 20)
    fig.tight_layout()
    out_path4 = os.path.join(OUTPUT_DIR, 'selfish_block_time.png')
    fig.savefig(out_path4, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {out_path4}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

if __name__ == '__main__':
    print(f"Parsing JSONL logs from: {OUTPUT_DIR}\n")

    df_exp_a = load_data(os.path.join(OUTPUT_DIR, 'metrics_exp_a.jsonl'))
    df_exp_b = load_data(os.path.join(OUTPUT_DIR, 'metrics_exp_b.jsonl'))

    print("\nGenerating plots...")
    plot_exp_a_generation_time(df_exp_a)
    plot_exp_b_difficulty(df_exp_b)
    plot_exp_b_generation_time(df_exp_b)
    plot_a_vs_b_block_time(df_exp_a, df_exp_b)
    plot_selfish_results()

    print("\n✅ All plots generated. Check the results/ directory.")