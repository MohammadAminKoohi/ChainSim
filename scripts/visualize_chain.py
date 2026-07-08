import json
import sys
import os
import matplotlib.pyplot as plt
import networkx as nx

def load_chain(jsonl_file):
    blocks = {} # hash -> {miner, parent, height, diff}
    tip = None
    
    with open(jsonl_file, 'r') as f:
        for line in f:
            if not line.strip(): continue
            event = json.loads(line)
            if event["type"] == "block_mined":
                data = event["data"]
                blocks[data["hash"]] = data
            elif event["type"] == "tip_updated":
                tip = event["data"]["new_tip"]
                
    return blocks, tip

def visualize(jsonl_file, out_file):
    blocks, tip = load_chain(jsonl_file)
    if not blocks:
        print("No blocks found.")
        return
        
    # Build main chain set
    main_chain = set()
    curr = tip
    while curr in blocks:
        main_chain.add(curr)
        curr = blocks[curr]["parent"]
        
    # Build graph
    G = nx.DiGraph()
    for h, b in blocks.items():
        G.add_node(h, miner=b["miner_id"], height=b["height"])
        if b["parent"] and b["parent"] in blocks:
            G.add_edge(b["parent"], h)
            
    # Assign positions
    pos = {}
    y_offsets = {} # height -> next available y
    
    # We must process nodes in topological order to place them correctly
    try:
        nodes = list(nx.topological_sort(G))
    except nx.NetworkXUnfeasible:
        # If there's a cycle (shouldn't happen), just use nodes
        nodes = list(G.nodes())
        
    for h in nodes:
        height = blocks[h]["height"]
        if h in main_chain:
            pos[h] = (height, 0)
        else:
            y = y_offsets.get(height, 1)
            # Alternate positive and negative offsets for forks
            actual_y = y if y % 2 != 0 else -y + 1
            pos[h] = (height, actual_y)
            y_offsets[height] = y + 1
            
    # Color mapping
    colors = []
    for h in G.nodes():
        miner = blocks[h]["miner_id"]
        if h in main_chain:
            colors.append('green' if miner == 'M1' else 'red')
        else:
            colors.append('lightgreen' if miner == 'M1' else 'lightcoral')
            
    plt.figure(figsize=(24, 6))
    
    # Draw graph
    nx.draw(G, pos, node_color=colors, node_size=30, width=0.5, with_labels=False, arrows=True, arrowsize=5, alpha=0.9)
    
    # Legend
    import matplotlib.lines as mlines
    m1_main = mlines.Line2D([], [], color='green', marker='o', linestyle='None', markersize=8, label='M1 (Honest) Main Chain')
    m2_main = mlines.Line2D([], [], color='red', marker='o', linestyle='None', markersize=8, label='M2 (Attacker) Main Chain')
    m1_orph = mlines.Line2D([], [], color='lightgreen', marker='o', linestyle='None', markersize=8, label='M1 Orphan / Fork')
    m2_orph = mlines.Line2D([], [], color='lightcoral', marker='o', linestyle='None', markersize=8, label='M2 Orphan / Fork')
    plt.legend(handles=[m1_main, m2_main, m1_orph, m2_orph], loc='upper left')
    
    plt.title(f"Blockchain Fork Visualization\n{len(main_chain)} Main Chain Blocks, {len(blocks) - len(main_chain)} Orphans/Forks")
    plt.xlabel("Block Height")
    
    # Customize axes to show height properly
    ax = plt.gca()
    ax.tick_params(left=False, bottom=True, labelleft=False, labelbottom=True)
    ax.set_axis_on()
    
    plt.savefig(out_file, dpi=300, bbox_inches='tight')
    plt.close()
    print(f"  ✓ Saved visualization: {out_file}")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python visualize_chain.py <path_to_jsonl>")
        sys.exit(1)
        
    jsonl_path = sys.argv[1]
    out_path = jsonl_path.replace('.jsonl', '_tree.png')
    visualize(jsonl_path, out_path)
