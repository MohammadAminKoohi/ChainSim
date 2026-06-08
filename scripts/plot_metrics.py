import json
import os
import pandas as pd
import matplotlib.pyplot as plt

# The directory where your Go simulator outputs the .jsonl files
OUTPUT_DIR = '../results' 

def load_data(filepath):
    """Loads JSON Lines into a structured pandas DataFrame."""
    if not os.path.exists(filepath):
        print(f"Skipping {filepath} - File not found.")
        return pd.DataFrame()

    data = []
    with open(filepath, 'r') as f:
        for line in f:
            if line.strip():
                data.append(json.loads(line))
    
    return pd.json_normalize(data)

def plot_exp_b_difficulty(df_b):
    """Plots Network Difficulty over time for Scenario B."""
    if df_b.empty: return

    blocks = df_b[df_b['type'] == 'block_mined'].copy()
    miners = df_b[df_b['type'] == 'miner_status'].copy()

    start_time = blocks['timestamp'].min()
    blocks['rel_time'] = (blocks['timestamp'] - start_time) / 1000.0

    plt.figure(figsize=(12, 6))
    plt.plot(blocks['rel_time'], blocks['data.difficulty'], drawstyle='steps-post', 
             color='#1f77b4', linewidth=2, label='Network Difficulty')

    for _, row in miners.iterrows():
        rel_time = (row['timestamp'] - start_time) / 1000.0
        if rel_time < 0: rel_time = 0 
            
        status = row['data.status']
        miner_id = row['data.miner_id']
        
        color = '#2ca02c' if status == 'joined' else '#d62728'
        plt.axvline(x=rel_time, color=color, linestyle='--', alpha=0.8)
        plt.text(rel_time + 0.5, plt.ylim()[1] * 0.95, f"{miner_id} {status}", 
                 rotation=90, verticalalignment='top', color=color, fontweight='bold')

    plt.title('Experiment B: Network Difficulty', fontsize=14)
    plt.xlabel('Simulated Time (seconds)', fontsize=12)
    plt.ylabel('Difficulty Multiplier', fontsize=12)
    plt.grid(True, linestyle=':', alpha=0.7)
    plt.legend(loc='upper left')
    plt.tight_layout()
    
    out_path = os.path.join(OUTPUT_DIR, 'exp_b_difficulty.png')
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"Saved: {out_path}")

def plot_exp_b_generation_time(df_b):
    """Plots the block generation time specifically for Scenario B."""
    if df_b.empty: return

    blocks = df_b[df_b['type'] == 'block_mined'].copy()
    blocks['production_time'] = blocks['timestamp'].diff() / 1000.0

    plt.figure(figsize=(12, 6))
    window = 5 
    
    plt.scatter(blocks['data.height'], blocks['production_time'], 
                color='#1f77b4', alpha=0.5, label='Raw Block Time')
    plt.plot(blocks['data.height'], blocks['production_time'].rolling(window=window, min_periods=1).mean(), 
             color='#000080', linewidth=2.5, label=f'{window}-Block Moving Avg')
    plt.axhline(y=1.0, color='red', linestyle='--', alpha=0.7, linewidth=1.5, label='Target Time (1.0s)')

    plt.title('Experiment B: Block Generation Time', fontsize=14)
    plt.xlabel('Block Height', fontsize=12)
    plt.ylabel('Generation Time (seconds)', fontsize=12)
    plt.grid(True, linestyle=':', alpha=0.7)
    plt.legend(loc='upper right')
    plt.tight_layout()
    
    out_path = os.path.join(OUTPUT_DIR, 'exp_b_generation_time.png')
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"Saved: {out_path}")

def plot_exp_a_generation_time(df_a):
    """Plots the block generation time for blocks 1 to 30 in Scenario A."""
    if df_a.empty: return

    blocks = df_a[df_a['type'] == 'block_mined'].copy()
    
    # Enforce the 1 to 30 block constraint
    blocks = blocks[blocks['data.height'] <= 30].copy()
    blocks['production_time'] = blocks['timestamp'].diff() / 1000.0

    plt.figure(figsize=(12, 6))
    
    # Using a line plot with markers to clearly show the sequence of the first 30 blocks
    plt.plot(blocks['data.height'], blocks['production_time'], 
             marker='o', linestyle='-', color='#ff7f0e', linewidth=2, markersize=6, label='Block Generation Time')
    plt.axhline(y=1.0, color='black', linestyle='--', alpha=0.7, linewidth=1.5, label='Target Time (1.0s)')

    plt.title('Experiment A: Block Generation Time (Blocks 1-30)', fontsize=14)
    plt.xlabel('Block Height', fontsize=12)
    plt.ylabel('Generation Time (seconds)', fontsize=12)
    plt.grid(True, linestyle=':', alpha=0.7)
    
    # Force x-axis to show integer ticks for the 30 blocks
    plt.xticks(range(0, 31, 2))
    
    plt.legend(loc='upper right')
    plt.tight_layout()
    
    out_path = os.path.join(OUTPUT_DIR, 'exp_a_generation_time.png')
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"Saved: {out_path}")

if __name__ == '__main__':
    print("Parsing JSONL files and generating isolated plots...")
    
    df_exp_a = load_data(os.path.join(OUTPUT_DIR, 'metrics_exp_a.jsonl'))
    df_exp_b = load_data(os.path.join(OUTPUT_DIR, 'metrics_exp_b.jsonl'))
    
    plot_exp_b_difficulty(df_exp_b)
    plot_exp_b_generation_time(df_exp_b)
    plot_exp_a_generation_time(df_exp_a)
    
    print("\nProcess Complete. You can now compare the images in your results directory.")