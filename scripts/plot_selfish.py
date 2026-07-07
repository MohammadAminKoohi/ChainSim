"""
ChainSim Selfish Mining Visualization Script
Parses results/selfish_results.json and generates plots comparing metrics across different values of R.
"""

import json
import os
import matplotlib.pyplot as plt
import numpy as np

# Directory paths
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
RESULTS_DIR = os.path.join(BASE_DIR, '..', 'results')
RESULTS_FILE = os.path.join(RESULTS_DIR, 'selfish_results.json')

# Styling configuration
plt.rcParams.update({
    'figure.facecolor': 'white',
    'axes.facecolor': '#fafafa',
    'axes.grid': True,
    'grid.alpha': 0.3,
    'grid.linestyle': ':',
    'font.size': 11,
})

def load_selfish_results():
    if not os.path.exists(RESULTS_FILE):
        print(f"Error: {RESULTS_FILE} not found. Run Go simulator first.")
        return []
    with open(RESULTS_FILE, 'r') as f:
        return json.load(f)

def plot_metrics(results):
    if not results:
        return

    # Extract data
    R = [r['r'] for r in results]
    avg_block_time = [r['avg_block_time_ms'] for r in results]
    attacker_share = [r['attacker_share'] * 100 for r in results]
    chain_quality = [r['chain_quality'] * 100 for r in results]
    forks = [r['forks'] for r in results]
    discarded = [r['discarded_blocks'] for r in results]
    waste_rate = [r['waste_rate'] * 100 for r in results]

    # --- Plot 1: Attacker Share & Chain Quality vs R ---
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
    plot_path1 = os.path.join(RESULTS_DIR, 'selfish_attacker_share.png')
    fig.savefig(plot_path1, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {plot_path1}")

    # --- Plot 2: Waste Rate and Discarded Blocks vs R ---
    fig, ax1 = plt.subplots(figsize=(10, 5.5))
    color = '#1f77b4'
    ax1.set_xlabel("Epoch Length R (Blocks between difficulty retargets)", labelpad=10)
    ax1.set_ylabel("Discarded (Orphan) Blocks Count", color=color, labelpad=10)
    bars = ax1.bar(R, discarded, color=color, alpha=0.4, width=2.0, label="Discarded Blocks")
    ax1.tick_params(axis='y', labelcolor=color)
    ax1.set_xticks(R)

    # Label heights of bars
    for bar in bars:
        height = bar.get_height()
        ax1.annotate(f'{int(height)}',
                     xy=(bar.get_x() + bar.get_width() / 2, height),
                     xytext=(0, 3),  # 3 points vertical offset
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=9, color=color, fontweight='bold')

    ax2 = ax1.twinx()  
    color = '#ff7f0e'
    ax2.set_ylabel("Network Hash Power Waste Rate (%)", color=color, labelpad=10)
    ax2.plot(R, waste_rate, color=color, marker='o', linewidth=2.5, label="Waste Rate (%)")
    ax2.tick_params(axis='y', labelcolor=color)
    ax2.set_ylim(0, max(waste_rate) * 1.3 if max(waste_rate) > 0 else 10)

    # Label line points
    for x, y in zip(R, waste_rate):
        ax2.annotate(f'{y:.2f}%',
                     xy=(x, y),
                     xytext=(0, 8),
                     textcoords="offset points",
                     ha='center', va='bottom', fontsize=9, color=color, fontweight='bold')

    plt.title("Orphan Blocks & Hash Power Waste Rate vs. Retargeting Interval (R)", fontsize=13, fontweight='bold', pad=15)
    fig.tight_layout()
    plot_path2 = os.path.join(RESULTS_DIR, 'selfish_waste_rate.png')
    fig.savefig(plot_path2, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {plot_path2}")

    # --- Plot 3: Forks vs R ---
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
    plot_path3 = os.path.join(RESULTS_DIR, 'selfish_forks.png')
    fig.savefig(plot_path3, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {plot_path3}")

    # --- Plot 4: Average Block Generation Time vs R ---
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
    plot_path4 = os.path.join(RESULTS_DIR, 'selfish_block_time.png')
    fig.savefig(plot_path4, dpi=300)
    plt.close(fig)
    print(f"  ✓ Saved: {plot_path4}")

if __name__ == '__main__':
    print("Parsing selfish mining results and generating charts...")
    data = load_selfish_results()
    plot_metrics(data)
    print("\n✅ All plots successfully generated. Check the results/ directory.")
