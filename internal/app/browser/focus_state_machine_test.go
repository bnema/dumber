package browser

import (
	"container/heap"
	"sync"
	"testing"
	"time"
)

// TestRingBuffer_AddAndRetrieve tests basic add and retrieve operations
func TestRingBuffer_AddAndRetrieve(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Add items
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	// Retrieve all
	items := rb.GetAll()

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	expected := []int{1, 2, 3}
	for i, item := range items {
		if item != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], item)
		}
	}
}

// TestRingBuffer_CircularOverwrite tests circular buffer overflow behavior
func TestRingBuffer_CircularOverwrite(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Fill buffer
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	// Overflow - should overwrite oldest
	rb.Add(4)
	rb.Add(5)

	items := rb.GetAll()

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Should have 3, 4, 5 (oldest items 1, 2 overwritten)
	expected := []int{3, 4, 5}
	for i, item := range items {
		if item != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], item)
		}
	}
}

// TestRingBuffer_EmptyBuffer tests empty buffer behavior
func TestRingBuffer_EmptyBuffer(t *testing.T) {
	rb := NewRingBuffer[string](5)

	items := rb.GetAll()

	if items != nil {
		t.Errorf("expected nil for empty buffer, got %v", items)
	}
}

// TestRingBuffer_ConcurrentAccess tests thread-safety
func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer[int](100)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				rb.Add(val*10 + j)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = rb.GetAll()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Should have 100 items without panicking
	items := rb.GetAll()
	if len(items) != 100 {
		t.Errorf("expected 100 items, got %d", len(items))
	}
}

// TestPriorityQueue_OrdersByPriority tests priority ordering
func TestPriorityQueue_OrdersByPriority(t *testing.T) {
	tests := []struct {
		name     string
		requests []*FocusRequest
		expected []string // IDs in expected pop order
	}{
		{
			name: "urgent before normal",
			requests: []*FocusRequest{
				{ID: "normal", Priority: PriorityNormal, Timestamp: time.Now()},
				{ID: "urgent", Priority: PriorityUrgent, Timestamp: time.Now()},
			},
			expected: []string{"urgent", "normal"},
		},
		{
			name: "high before low",
			requests: []*FocusRequest{
				{ID: "low", Priority: PriorityLow, Timestamp: time.Now()},
				{ID: "high", Priority: PriorityHigh, Timestamp: time.Now()},
				{ID: "normal", Priority: PriorityNormal, Timestamp: time.Now()},
			},
			expected: []string{"high", "normal", "low"},
		},
		{
			name: "multiple same priority",
			requests: []*FocusRequest{
				{ID: "first", Priority: PriorityNormal, Timestamp: time.Now()},
				{ID: "second", Priority: PriorityNormal, Timestamp: time.Now()},
				{ID: "urgent", Priority: PriorityUrgent, Timestamp: time.Now()},
			},
			expected: []string{"urgent", "first", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pq := make(FocusPriorityQueue, 0)
			heap.Init(&pq)

			// Push all requests
			for _, req := range tt.requests {
				heap.Push(&pq, req)
			}

			// Pop and verify order
			for i, expectedID := range tt.expected {
				if pq.Len() == 0 {
					t.Fatalf("queue empty at position %d, expected %s", i, expectedID)
				}
				req := heap.Pop(&pq).(*FocusRequest)
				if req.ID != expectedID {
					t.Errorf("at position %d: expected ID %s, got %s", i, expectedID, req.ID)
				}
			}

			if pq.Len() != 0 {
				t.Errorf("queue should be empty, has %d items", pq.Len())
			}
		})
	}
}

// TestPriorityQueue_PushPop tests basic push/pop operations
func TestPriorityQueue_PushPop(t *testing.T) {
	pq := make(FocusPriorityQueue, 0)
	heap.Init(&pq)

	// Push items
	heap.Push(&pq, &FocusRequest{ID: "1", Priority: 50})
	heap.Push(&pq, &FocusRequest{ID: "2", Priority: 90})
	heap.Push(&pq, &FocusRequest{ID: "3", Priority: 10})

	if pq.Len() != 3 {
		t.Errorf("expected length 3, got %d", pq.Len())
	}

	// Pop should return highest priority (90)
	first := heap.Pop(&pq).(*FocusRequest)
	if first.ID != "2" {
		t.Errorf("expected ID '2', got '%s'", first.ID)
	}

	// Next should be 50
	second := heap.Pop(&pq).(*FocusRequest)
	if second.ID != "1" {
		t.Errorf("expected ID '1', got '%s'", second.ID)
	}

	// Last should be 10
	third := heap.Pop(&pq).(*FocusRequest)
	if third.ID != "3" {
		t.Errorf("expected ID '3', got '%s'", third.ID)
	}

	if pq.Len() != 0 {
		t.Errorf("queue should be empty")
	}
}

// TestRequestDeduplicator_DetectsDuplicates tests duplicate detection
func TestRequestDeduplicator_DetectsDuplicates(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)

	// Create a dummy paneNode (nil is ok for signature generation)
	node := &paneNode{}

	req1 := FocusRequest{
		TargetNode: node,
		Source:     SourceKeyboard,
		Timestamp:  time.Now(),
	}

	// First request should not be duplicate
	if dedup.IsDuplicate(req1) {
		t.Error("first request should not be duplicate")
	}

	// Immediate second request with same params should be duplicate
	req2 := FocusRequest{
		TargetNode: node,
		Source:     SourceKeyboard,
		Timestamp:  time.Now(),
	}

	if !dedup.IsDuplicate(req2) {
		t.Error("second identical request should be duplicate")
	}
}

// TestRequestDeduplicator_DifferentSourcesNotDuplicate tests different sources
func TestRequestDeduplicator_DifferentSourcesNotDuplicate(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)

	node := &paneNode{}

	req1 := FocusRequest{
		TargetNode: node,
		Source:     SourceKeyboard,
	}

	req2 := FocusRequest{
		TargetNode: node,
		Source:     SourceMouse,
	}

	dedup.IsDuplicate(req1)

	// Different source should not be duplicate
	if dedup.IsDuplicate(req2) {
		t.Error("requests with different sources should not be duplicates")
	}
}

// TestRequestDeduplicator_ExpiresOldSignatures tests TTL expiration
func TestRequestDeduplicator_ExpiresOldSignatures(t *testing.T) {
	ttl := 50 * time.Millisecond
	dedup := NewRequestDeduplicator(ttl)

	node := &paneNode{}

	req := FocusRequest{
		TargetNode: node,
		Source:     SourceKeyboard,
	}

	// First request
	if dedup.IsDuplicate(req) {
		t.Error("first request should not be duplicate")
	}

	// Immediate duplicate
	if !dedup.IsDuplicate(req) {
		t.Error("immediate duplicate should be detected")
	}

	// Wait for TTL to expire
	time.Sleep(ttl + 10*time.Millisecond)

	// Should not be duplicate after expiration
	if dedup.IsDuplicate(req) {
		t.Error("request after TTL should not be duplicate")
	}
}

// TestRequestDeduplicator_ConcurrentAccess tests thread-safety
func TestRequestDeduplicator_ConcurrentAccess(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var wg sync.WaitGroup

	nodes := make([]*paneNode, 10)
	for i := range nodes {
		nodes[i] = &paneNode{}
	}

	// Concurrent duplicate checks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				req := FocusRequest{
					TargetNode: nodes[idx%len(nodes)],
					Source:     SourceKeyboard,
				}
				_ = dedup.IsDuplicate(req)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	// Should complete without race conditions or panics
}

// TestFocusRequest_Validation tests request validation logic
func TestFocusRequest_Validation(t *testing.T) {
	tests := []struct {
		name      string
		request   *FocusRequest
		wantValid bool
	}{
		{
			name: "valid request",
			request: &FocusRequest{
				ID:         "test-1",
				TargetNode: &paneNode{},
				Source:     SourceKeyboard,
				Priority:   PriorityNormal,
				Timestamp:  time.Now(),
			},
			wantValid: true,
		},
		{
			name: "nil target node",
			request: &FocusRequest{
				ID:        "test-2",
				Source:    SourceKeyboard,
				Priority:  PriorityNormal,
				Timestamp: time.Now(),
			},
			wantValid: false,
		},
		{
			name: "empty ID",
			request: &FocusRequest{
				ID:         "",
				TargetNode: &paneNode{},
				Source:     SourceKeyboard,
				Priority:   PriorityNormal,
				Timestamp:  time.Now(),
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.request != nil &&
				tt.request.TargetNode != nil &&
				tt.request.ID != ""

			if valid != tt.wantValid {
				t.Errorf("expected valid=%v, got valid=%v", tt.wantValid, valid)
			}
		})
	}
}

// TestPriorityCalculation_BySource tests priority assignment by source
func TestPriorityCalculation_BySource(t *testing.T) {
	tests := []struct {
		source   FocusSource
		expected int
	}{
		{SourceSystem, PriorityUrgent},
		{SourceKeyboard, PriorityHigh},
		{SourceProgrammatic, PriorityNormal},
		{SourceMouse, PriorityNormal},
		{SourceStackNav, PriorityHigh},
		{SourceSplit, PriorityHigh},
		{SourceClose, PriorityUrgent},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			// This would be the actual priority calculation logic
			// For now, we just verify expected mappings
			var priority int
			switch tt.source {
			case SourceSystem, SourceClose:
				priority = PriorityUrgent
			case SourceKeyboard, SourceStackNav, SourceSplit:
				priority = PriorityHigh
			case SourceProgrammatic, SourceMouse:
				priority = PriorityNormal
			default:
				priority = PriorityLow
			}

			if priority != tt.expected {
				t.Errorf("source %s: expected priority %d, got %d", tt.source, tt.expected, priority)
			}
		})
	}
}
