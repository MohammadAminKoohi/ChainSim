package core

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func createTestBlock(parentHash string, difficulty float64, timestamp int64, nonce uint64, body string) *Block {
	digest := sha256.Sum256([]byte(body))
	return &Block{
		Header: BlockHeader{
			Parent:     parentHash,
			Digest:     hex.EncodeToString(digest[:]),
			Difficulty: difficulty,
			Timestamp:  timestamp,
			Nonce:      nonce,
		},
		Body: body,
	}
}

// helper to find a valid nonce for a test block
func mineBlockHelper(b *Block) {
	for {
		hash := b.Hash()
		if IsValidPoW(hash, b.Header.Difficulty) {
			return
		}
		b.Header.Nonce++
	}
}

func TestNewBlockChain(t *testing.T) {
	genesis := createTestBlock("", 1.0, 1000, 0, "Genesis")
	mineBlockHelper(genesis)

	bc, err := NewBlockChain(genesis)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	tip := bc.GetHeaviestChainTip()
	if tip == nil {
		t.Fatal("heaviest tip is nil")
	}
	if tip.Hash != genesis.Hash() {
		t.Errorf("expected tip hash %s, got %s", genesis.Hash(), tip.Hash)
	}
}

func TestAddBlockInvalidPoW(t *testing.T) {
	genesis := createTestBlock("", 1.0, 1000, 0, "Genesis")
	mineBlockHelper(genesis)

	bc, err := NewBlockChain(genesis)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	// Create block with extremely high difficulty but nonce = 0 (unmined)
	invalidBlock := createTestBlock(genesis.Hash(), 1000000.0, 2000, 0, "Invalid")
	// Note: We don't mine it, so with high probability its nonce = 0 hash will fail IsValidPoW at difficulty 10.0.
	err = bc.AddBlock(invalidBlock)
	if err == nil {
		t.Error("expected error adding block with invalid PoW, but got nil")
	}
}

func TestCalculateNextDifficulty(t *testing.T) {
	// DifficultyEpoch is 3.
	// TargetTimePerBlock is 1000ms.
	genesis := createTestBlock("", 1.0, 1000, 0, "Genesis")
	mineBlockHelper(genesis)

	bc, err := NewBlockChain(genesis)
	if err != nil {
		t.Fatalf("failed to create blockchain: %v", err)
	}

	// Height 1 block: building on genesis (height 0)
	// nextHeight will be 1. 1 % 3 != 0, so difficulty should remain 1.0.
	diff, err := bc.CalculateNextDifficulty(genesis.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 1.0 {
		t.Errorf("expected difficulty 1.0 at height 1, got %f", diff)
	}

	// Add block 1 (height 1) at timestamp 2000
	b1 := createTestBlock(genesis.Hash(), 1.0, 2000, 0, "Block 1")
	mineBlockHelper(b1)
	if err := bc.AddBlock(b1); err != nil {
		t.Fatalf("failed to add block 1: %v", err)
	}

	// Height 2 block: building on b1 (height 1)
	// nextHeight will be 2. 2 % 3 != 0, so difficulty should remain 1.0.
	diff, err = bc.CalculateNextDifficulty(b1.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 1.0 {
		t.Errorf("expected difficulty 1.0 at height 2, got %f", diff)
	}

	// Add block 2 (height 2) at timestamp 3000
	b2 := createTestBlock(b1.Hash(), 1.0, 3000, 0, "Block 2")
	mineBlockHelper(b2)
	if err := bc.AddBlock(b2); err != nil {
		t.Fatalf("failed to add block 2: %v", err)
	}

	// Height 3 block: building on b2 (height 2)
	// nextHeight will be 3. 3 % 3 == 0, so difficulty should adjust!
	// Ancestor will be genesis (height 0).
	// actualDuration = Timestamp(b2) - Timestamp(genesis) = 3000 - 1000 = 2000ms.
	// numIntervals = 2 - 0 = 2.
	// targetDuration = 2 * 1000 = 2000ms.
	// ratio = 2000 / 2000 = 1.0.
	// Expected next difficulty: 1.0 * 1.0 = 1.0.
	diff, err = bc.CalculateNextDifficulty(b2.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 1.0 {
		t.Errorf("expected difficulty 1.0 at height 3, got %f", diff)
	}

	// Let's test a case where difficulty adjusts upwards.
	// Suppose b2 was mined very quickly at timestamp 1500 instead of 3000.
	// Then actualDuration = 1500 - 1000 = 500ms.
	// targetDuration = 2 * 1000 = 2000ms.
	// ratio = 2000 / 500 = 4.0.
	// Since MaxAdjustmentFactor = 2, ratio is clamped to 2.0.
	// Expected next difficulty: 1.0 * 2.0 = 2.0.
	bcFast, _ := NewBlockChain(genesis)
	b1Fast := createTestBlock(genesis.Hash(), 1.0, 1200, 0, "Block 1 Fast")
	mineBlockHelper(b1Fast)
	_ = bcFast.AddBlock(b1Fast)
	b2Fast := createTestBlock(b1Fast.Hash(), 1.0, 1500, 0, "Block 2 Fast")
	mineBlockHelper(b2Fast)
	_ = bcFast.AddBlock(b2Fast)

	diff, err = bcFast.CalculateNextDifficulty(b2Fast.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 2.0 {
		t.Errorf("expected difficulty 2.0 (clamped) at height 3, got %f", diff)
	}

	// Let's test a case where difficulty adjusts downwards.
	// Suppose b2 was mined slowly at timestamp 6000.
	// actualDuration = 6000 - 1000 = 5000ms.
	// targetDuration = 2 * 1000 = 2000ms.
	// ratio = 2000 / 5000 = 0.4.
	// Since MinAdjustmentFactor = 0.5, ratio is clamped to 0.5.
	// Expected next difficulty: 1.0 * 0.5 = 0.5.
	// BUT absolute floor for difficulty is 1.0, so difficulty should remain 1.0.
	// Let's start with a higher difficulty to see clamping in action.
	genesisHigh := createTestBlock("", 10.0, 1000, 0, "Genesis High")
	mineBlockHelper(genesisHigh)
	bcSlow, _ := NewBlockChain(genesisHigh)
	b1Slow := createTestBlock(genesisHigh.Hash(), 10.0, 4000, 0, "Block 1 Slow")
	mineBlockHelper(b1Slow)
	_ = bcSlow.AddBlock(b1Slow)
	b2Slow := createTestBlock(b1Slow.Hash(), 10.0, 7000, 0, "Block 2 Slow")
	mineBlockHelper(b2Slow)
	_ = bcSlow.AddBlock(b2Slow)

	// actualDuration = 7000 - 1000 = 6000ms.
	// targetDuration = 2 * 1000 = 2000ms.
	// ratio = 2000 / 6000 = 0.3333.
	// ratio clamped to MinAdjustmentFactor = 0.5.
	// Expected next difficulty: 10.0 * 0.5 = 5.0.
	diff, err = bcSlow.CalculateNextDifficulty(b2Slow.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 5.0 {
		t.Errorf("expected difficulty 5.0 (clamped) at height 3, got %f", diff)
	}
}

func TestCalculateNextDifficultySubsequentEpoch(t *testing.T) {
	// Let's test difficulty adjustment for subsequent epochs (no genesis boundary).
	genesis := createTestBlock("", 1.0, 1000, 0, "Genesis")
	mineBlockHelper(genesis)
	bc, _ := NewBlockChain(genesis)

	// Add blocks to reach height 3
	b1 := createTestBlock(genesis.Hash(), 1.0, 2000, 0, "B1")
	mineBlockHelper(b1)
	_ = bc.AddBlock(b1)
	b2 := createTestBlock(b1.Hash(), 1.0, 3000, 0, "B2")
	mineBlockHelper(b2)
	_ = bc.AddBlock(b2)

	// Height 3 block gets difficulty 1.0
	b3 := createTestBlock(b2.Hash(), 1.0, 4000, 0, "B3")
	mineBlockHelper(b3)
	_ = bc.AddBlock(b3)

	// Add blocks 4 and 5
	b4 := createTestBlock(b3.Hash(), 1.0, 5000, 0, "B4")
	mineBlockHelper(b4)
	_ = bc.AddBlock(b4)
	b5 := createTestBlock(b4.Hash(), 1.0, 5500, 0, "B5")
	mineBlockHelper(b5)
	_ = bc.AddBlock(b5)

	// Height 6 block: building on b5 (height 5). 6 % 3 == 0.
	// Walk back 3 steps: b5 -> b4 -> b3 -> b2.
	// Ancestor is b2 (height 2).
	// numIntervals = 5 - 2 = 3.
	// targetDuration = 3 * 1000 = 3000ms.
	// actualDuration = Timestamp(b5) - Timestamp(b2) = 5500 - 3000 = 2500ms.
	// ratio = 3000 / 2500 = 1.2.
	// Expected next difficulty: 1.0 * 1.2 = 1.2.
	diff, err := bc.CalculateNextDifficulty(b5.Hash())
	if err != nil {
		t.Fatalf("CalculateNextDifficulty failed: %v", err)
	}
	if diff != 1.2 {
		t.Errorf("expected difficulty 1.2 at height 6, got %f", diff)
	}
}
