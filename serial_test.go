package modbusone

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestStats_Clone(t *testing.T) {
	// 1. Initialize every single field with a distinct value
	orig := &Stats{
		ReadPackets:      1,
		CrcErrors:        2,
		RemoteErrors:     3,
		OtherErrors:      4,
		LongReadWarnings: 5,
		FormateWarnings:  6, // Keeping typo for compatibility
		IDDrops:          7,
		OtherDrops:       8,
	}

	cloned := orig.Clone()

	// 2. Verify it is a new memory address
	if cloned == orig {
		t.Error("Clone returned the same memory pointer, expected a new instance")
	}

	// 3. Exhaustively check every field value
	if cloned.ReadPackets != 1 {
		t.Errorf("ReadPackets mismatch: got %d, want 1", cloned.ReadPackets)
	}
	if cloned.CrcErrors != 2 {
		t.Errorf("CrcErrors mismatch: got %d, want 2", cloned.CrcErrors)
	}
	if cloned.RemoteErrors != 3 {
		t.Errorf("RemoteErrors mismatch: got %d, want 3", cloned.RemoteErrors)
	}
	if cloned.OtherErrors != 4 {
		t.Errorf("OtherErrors mismatch: got %d, want 4", cloned.OtherErrors)
	}
	if cloned.LongReadWarnings != 5 {
		t.Errorf("LongReadWarnings mismatch: got %d, want 5", cloned.LongReadWarnings)
	}
	if cloned.FormateWarnings != 6 {
		t.Errorf("FormateWarnings mismatch: got %d, want 6", cloned.FormateWarnings)
	}
	if cloned.IDDrops != 7 {
		t.Errorf("IDDrops mismatch: got %d, want 7", cloned.IDDrops)
	}
	if cloned.OtherDrops != 8 {
		t.Errorf("OtherDrops mismatch: got %d, want 8", cloned.OtherDrops)
	}
}

func TestStats_Reset(t *testing.T) {
	s := &Stats{
		ReadPackets:      10,
		CrcErrors:        20,
		RemoteErrors:     30,
		OtherErrors:      40,
		LongReadWarnings: 50,
		FormateWarnings:  60,
		IDDrops:          70,
		OtherDrops:       80,
	}

	s.Reset()

	// Exhaustively verify every field was cleared to 0
	if s.ReadPackets != 0 || s.CrcErrors != 0 || s.RemoteErrors != 0 || s.OtherErrors != 0 ||
		s.LongReadWarnings != 0 || s.FormateWarnings != 0 || s.IDDrops != 0 || s.OtherDrops != 0 {
		t.Errorf("Reset failed to zero out all fields. Got: %+v", s)
	}
}

func TestStats_TotalDrops(t *testing.T) {
	s := &Stats{
		ReadPackets:      999, // Should NOT be included in TotalDrops calculation
		CrcErrors:        1,
		RemoteErrors:     2,
		OtherErrors:      3,
		LongReadWarnings: 4,
		FormateWarnings:  5,
		IDDrops:          6,
		OtherDrops:       7,
	}

	expected := int64(1 + 2 + 3 + 4 + 5 + 6 + 7)
	if got := s.TotalDrops(); got != expected {
		t.Errorf("TotalDrops() = %d; want %d", got, expected)
	}
}

func TestStats_String(t *testing.T) {
	s := &Stats{
		ReadPackets:      100, // String() method omits ReadPackets based on your original code
		CrcErrors:        1,
		RemoteErrors:     2,
		OtherErrors:      3,
		LongReadWarnings: 4,
		FormateWarnings:  5,
		IDDrops:          6,
		OtherDrops:       7,
	}

	// fmt.Sprint formats fields separated by spaces
	expected := fmt.Sprint(1, 2, 3, 4, 5, 6, 7)
	if got := s.String(); got != expected {
		t.Errorf("String() mismatch.\nGot : %q\nWant: %q", got, expected)
	}
}

// TestStats_Concurrency runs parallel operations under stress
func TestStats_Concurrency(t *testing.T) {
	s := &Stats{}
	var wg sync.WaitGroup
	workers := 10
	iterations := 500

	for i := 0; i < workers; i++ {
		wg.Add(2)
		// Writer routine
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				atomic.AddInt64(&s.ReadPackets, 1)
				s.Reset()
			}
		}()
		// Reader routine
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = s.Clone()
				_ = s.TotalDrops()
				_ = s.String()
			}
		}()
	}
	wg.Wait()
}
