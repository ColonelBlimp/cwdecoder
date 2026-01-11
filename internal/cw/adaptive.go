// internal/cw/adaptive.go
// Package cw implements adaptive CW decoding with dictionary-based timing correction.
package cw

import (
	"strings"
	"sync"
	"time"
)

// Element timing constants
const (
	// MaxElementBuffer is the maximum number of elements to keep in the buffer
	MaxElementBuffer = 50
	// MinPatternConfidence is the minimum confidence score to trigger adjustment (0.0-1.0)
	MinPatternConfidence = 0.7
	// MaxPatternLength is the maximum number of characters in a pattern to match
	MaxPatternLength = 8
	// MinMatchesForAdjustment is the minimum pattern matches before adjusting timing
	MinMatchesForAdjustment = 3
	// AdaptiveAdjustmentRate is how fast to adjust inter_char_boundary (EMA factor)
	AdaptiveAdjustmentRate = 0.1
	// GapToleranceRatio is the tolerance for gap duration matching (e.g., 0.5 = ±50%)
	GapToleranceRatio = 0.5
)

// Element represents a single Morse code element (dit or dah)
type Element struct {
	IsDah     bool          // true for dah, false for dit
	Duration  time.Duration // duration of the tone
	GapAfter  time.Duration // silence duration after this element
	Timestamp time.Time     // when the element occurred
	IsCharEnd bool          // true if this element ends a character (based on current threshold)
	IsWordEnd bool          // true if this element ends a word
}

// MorsePattern represents a known Morse code pattern
type MorsePattern struct {
	Text     string // The decoded text (e.g., "CQ")
	Elements []bool // Element pattern: false=dit, true=dah (e.g., [true,false,true,false, true,true,false,true])
	Breaks   []int  // Indices where character breaks should occur (after element at index)
	Priority int    // Higher priority patterns are matched first
}

// CommonPatterns contains frequently used CW words and phrases
// Break indices are 0-based positions where character boundaries occur (after that element)
var CommonPatterns = []MorsePattern{
	// High priority - very common multi-character patterns only
	// CQ = -.-. --.- (C=dah dit dah dit, Q=dah dah dit dah)
	{Text: "CQ", Elements: []bool{true, false, true, false, true, true, false, true}, Breaks: []int{3}, Priority: 10},
	// DE = -.. . (D=dah dit dit, E=dit)
	{Text: "DE", Elements: []bool{true, false, false, false}, Breaks: []int{2}, Priority: 10},
	// 73 = --... ...-- (7=dah dah dit dit dit, 3=dit dit dit dah dah)
	{Text: "73", Elements: []bool{true, true, false, false, false, false, false, false, true, true}, Breaks: []int{4}, Priority: 9},
	// 5NN = ..... -. -. (5=dit dit dit dit dit, N=dah dit, N=dah dit)
	{Text: "5NN", Elements: []bool{false, false, false, false, false, true, false, true, false}, Breaks: []int{4, 6}, Priority: 9},
	// 599 = ..... ----. ----.  (5=....., 9=----.)
	{Text: "599", Elements: []bool{false, false, false, false, false, true, true, true, true, false, true, true, true, true, false}, Breaks: []int{4, 9}, Priority: 8},

	// Q codes - Q = --.- (4 elements)
	// QTH = --.- - .... (Q=dah dah dit dah, T=dah, H=dit dit dit dit)
	{Text: "QTH", Elements: []bool{true, true, false, true, true, false, false, false, false}, Breaks: []int{3, 4}, Priority: 7},
	// QRZ = --.- .-. --.. (Q=dah dah dit dah, R=dit dah dit, Z=dah dah dit dit)
	{Text: "QRZ", Elements: []bool{true, true, false, true, false, true, false, true, true, false, false}, Breaks: []int{3, 6}, Priority: 7},
	// QSO = --.- ... --- (Q=dah dah dit dah, S=dit dit dit, O=dah dah dah)
	{Text: "QSO", Elements: []bool{true, true, false, true, false, false, false, true, true, true}, Breaks: []int{3, 6}, Priority: 7},
	// QSL = --.- ... .-.. (Q=dah dah dit dah, S=dit dit dit, L=dit dah dit dit)
	{Text: "QSL", Elements: []bool{true, true, false, true, false, false, false, false, true, false, false}, Breaks: []int{3, 6}, Priority: 7},

	// Common words - only multi-character patterns
	// TU = - ..- (T=dah, U=dit dit dah)
	{Text: "TU", Elements: []bool{true, false, false, true}, Breaks: []int{0}, Priority: 8},
	// GM = --. -- (G=dah dah dit, M=dah dah)
	{Text: "GM", Elements: []bool{true, true, false, true, true}, Breaks: []int{2}, Priority: 7},
	// GA = --. .- (G=dah dah dit, A=dit dah)
	{Text: "GA", Elements: []bool{true, true, false, false, true}, Breaks: []int{2}, Priority: 7},
	// GE = --. . (G=dah dah dit, E=dit)
	{Text: "GE", Elements: []bool{true, true, false, false}, Breaks: []int{2}, Priority: 7},
	// UR = ..- .-. (U=dit dit dah, R=dit dah dit)
	{Text: "UR", Elements: []bool{false, false, true, false, true, false}, Breaks: []int{2}, Priority: 6},
	// FB = ..-. -... (F=dit dit dah dit, B=dah dit dit dit)
	{Text: "FB", Elements: []bool{false, false, true, false, true, false, false, false}, Breaks: []int{3}, Priority: 6},
	// ES = . ... (E=dit, S=dit dit dit)
	{Text: "ES", Elements: []bool{false, false, false, false}, Breaks: []int{0}, Priority: 6},
	// HR = .... .-. (H=dit dit dit dit, R=dit dah dit)
	{Text: "HR", Elements: []bool{false, false, false, false, false, true, false}, Breaks: []int{3}, Priority: 5},
}

// PatternMatch represents a potential pattern match in the element buffer
type PatternMatch struct {
	Pattern    *MorsePattern
	StartIndex int     // Index in element buffer where match starts
	EndIndex   int     // Index in element buffer where match ends
	Confidence float64 // Match confidence (0.0-1.0)
	// Suggested timing adjustment
	SuggestedInterCharBoundary float64
}

// AdaptiveDecoder wraps the base Decoder with pattern matching and adaptive timing
type AdaptiveDecoder struct {
	decoder *Decoder
	config  AdaptiveConfig

	mu             sync.Mutex
	elementBuffer  []Element
	patternMatches map[string]int // Count of pattern matches for confidence building

	// Callback for pattern-corrected output
	correctedCallback CorrectedCallback
}

// AdaptiveConfig holds configuration for adaptive decoding
type AdaptiveConfig struct {
	// Enabled turns on adaptive pattern matching
	Enabled bool
	// MinConfidence is the minimum confidence score to trigger adjustment (from config: adaptive_min_confidence)
	MinConfidence float64
	// AdjustmentRate is the EMA rate for timing adjustments (from config: adaptive_adjustment_rate)
	AdjustmentRate float64
	// MinMatchesForAdjust is how many matches before adjusting (from config: adaptive_min_matches)
	MinMatchesForAdjust int
}

// CorrectedOutput represents pattern-corrected decoded output
type CorrectedOutput struct {
	// Original is what the decoder produced
	Original string
	// Corrected is the pattern-matched correction (empty if no correction)
	Corrected string
	// Pattern is the matched pattern (nil if no match)
	Pattern *MorsePattern
	// Confidence is the match confidence
	Confidence float64
	// TimingAdjusted is true if timing was adjusted
	TimingAdjusted bool
}

// CorrectedCallback is called when pattern correction occurs
type CorrectedCallback func(output CorrectedOutput)

// NewAdaptiveDecoder creates a new adaptive decoder wrapping the base decoder
func NewAdaptiveDecoder(decoder *Decoder, config AdaptiveConfig) *AdaptiveDecoder {
	if config.MinConfidence <= 0 {
		config.MinConfidence = MinPatternConfidence
	}
	if config.AdjustmentRate <= 0 {
		config.AdjustmentRate = AdaptiveAdjustmentRate
	}
	if config.MinMatchesForAdjust <= 0 {
		config.MinMatchesForAdjust = MinMatchesForAdjustment
	}

	return &AdaptiveDecoder{
		decoder:        decoder,
		config:         config,
		elementBuffer:  make([]Element, 0, MaxElementBuffer),
		patternMatches: make(map[string]int),
	}
}

// SetCorrectedCallback sets the callback for pattern-corrected output
func (a *AdaptiveDecoder) SetCorrectedCallback(cb CorrectedCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.correctedCallback = cb
}

// RecordElement adds a new element to the buffer for pattern matching
func (a *AdaptiveDecoder) RecordElement(isDah bool, duration, gapAfter time.Duration, isCharEnd, isWordEnd bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	elem := Element{
		IsDah:     isDah,
		Duration:  duration,
		GapAfter:  gapAfter,
		Timestamp: time.Now(),
		IsCharEnd: isCharEnd,
		IsWordEnd: isWordEnd,
	}

	a.elementBuffer = append(a.elementBuffer, elem)

	// Trim buffer if too large
	if len(a.elementBuffer) > MaxElementBuffer {
		a.elementBuffer = a.elementBuffer[len(a.elementBuffer)-MaxElementBuffer:]
	}

	// Check for pattern matches after word boundaries
	if isWordEnd || isCharEnd {
		a.checkPatterns()
	}
}

// checkPatterns looks for known patterns in the element buffer
func (a *AdaptiveDecoder) checkPatterns() {
	if len(a.elementBuffer) < 2 {
		return
	}

	// Find the start of the current "word" (after last word boundary)
	startIdx := 0
	for i := len(a.elementBuffer) - 2; i >= 0; i-- {
		if a.elementBuffer[i].IsWordEnd {
			startIdx = i + 1
			break
		}
	}

	// Extract elements for current word/phrase
	elements := a.elementBuffer[startIdx:]
	if len(elements) < 2 {
		return
	}

	// Try to match patterns
	match := a.findBestMatch(elements)
	if match != nil && match.Confidence >= a.config.MinConfidence {
		a.handlePatternMatch(match, elements)
	}
}

// findBestMatch finds the best matching pattern for the given elements
func (a *AdaptiveDecoder) findBestMatch(elements []Element) *PatternMatch {
	var bestMatch *PatternMatch

	for i := range CommonPatterns {
		pattern := &CommonPatterns[i]
		if len(pattern.Elements) > len(elements) {
			continue
		}

		// Check if elements match the pattern
		match := a.matchPattern(pattern, elements)
		if match != nil {
			if bestMatch == nil ||
				match.Confidence > bestMatch.Confidence ||
				(match.Confidence == bestMatch.Confidence && pattern.Priority > bestMatch.Pattern.Priority) {
				bestMatch = match
			}
		}
	}

	return bestMatch
}

// matchPattern checks if elements match a pattern and calculates confidence
func (a *AdaptiveDecoder) matchPattern(pattern *MorsePattern, elements []Element) *PatternMatch {
	// Require exact element count match for reliable pattern matching
	if len(elements) != len(pattern.Elements) {
		return nil
	}

	// Check element types match (dit/dah) - require 100% match
	for i, isDah := range pattern.Elements {
		if elements[i].IsDah != isDah {
			return nil // Any mismatch = no match
		}
	}

	// Check if character breaks are in the right places
	breakConfidence := a.calculateBreakConfidence(pattern, elements)

	// Require high break confidence - breaks must align with pattern
	if breakConfidence < 0.8 {
		return nil
	}

	// Calculate suggested timing adjustment
	suggestedBoundary := a.calculateSuggestedBoundary(pattern, elements)

	// Overall confidence based on break alignment
	confidence := breakConfidence

	return &PatternMatch{
		Pattern:                    pattern,
		StartIndex:                 0,
		EndIndex:                   len(pattern.Elements) - 1,
		Confidence:                 confidence,
		SuggestedInterCharBoundary: suggestedBoundary,
	}
}

// calculateBreakConfidence checks if character breaks align with pattern expectations
func (a *AdaptiveDecoder) calculateBreakConfidence(pattern *MorsePattern, elements []Element) float64 {
	if len(pattern.Breaks) == 0 {
		return 1.0 // Single character, no breaks needed
	}

	correctBreaks := 0
	for _, breakIdx := range pattern.Breaks {
		if breakIdx < len(elements) && elements[breakIdx].IsCharEnd {
			correctBreaks++
		}
	}

	return float64(correctBreaks) / float64(len(pattern.Breaks))
}

// calculateSuggestedBoundary analyzes gaps to suggest optimal inter_char_boundary
func (a *AdaptiveDecoder) calculateSuggestedBoundary(pattern *MorsePattern, elements []Element) float64 {
	if len(pattern.Breaks) == 0 || len(elements) < 2 {
		return 0 // No suggestion
	}

	// Collect intra-character gaps and inter-character gaps
	var intraGaps, interGaps []float64
	ditDuration := a.decoder.ditDurationMs

	for i := 0; i < len(pattern.Elements)-1 && i < len(elements)-1; i++ {
		gapMs := float64(elements[i].GapAfter.Milliseconds())
		gapRatio := gapMs / ditDuration

		isBreak := false
		for _, breakIdx := range pattern.Breaks {
			if breakIdx == i {
				isBreak = true
				break
			}
		}

		if isBreak {
			interGaps = append(interGaps, gapRatio)
		} else {
			intraGaps = append(intraGaps, gapRatio)
		}
	}

	// Calculate optimal boundary (midpoint between max intra and min inter)
	if len(intraGaps) == 0 || len(interGaps) == 0 {
		return 0
	}

	maxIntra := 0.0
	for _, g := range intraGaps {
		if g > maxIntra {
			maxIntra = g
		}
	}

	minInter := interGaps[0]
	for _, g := range interGaps {
		if g < minInter {
			minInter = g
		}
	}

	// Suggested boundary is midpoint
	if minInter > maxIntra {
		return (maxIntra + minInter) / 2
	}

	return 0 // Gaps are overlapping, can't determine
}

// handlePatternMatch processes a successful pattern match
func (a *AdaptiveDecoder) handlePatternMatch(match *PatternMatch, elements []Element) {
	// Increment match counter
	a.patternMatches[match.Pattern.Text]++
	matchCount := a.patternMatches[match.Pattern.Text]

	// Build original decoded string from elements
	original := a.decodeElements(elements[:len(match.Pattern.Elements)])

	// Prepare corrected output
	output := CorrectedOutput{
		Original:       original,
		Corrected:      match.Pattern.Text,
		Pattern:        match.Pattern,
		Confidence:     match.Confidence,
		TimingAdjusted: false,
	}

	// Adjust timing if we have enough matches and a valid suggestion
	if matchCount >= a.config.MinMatchesForAdjust && match.SuggestedInterCharBoundary > 0 {
		currentBoundary := a.decoder.config.InterCharBoundary
		newBoundary := currentBoundary*(1-a.config.AdjustmentRate) +
			match.SuggestedInterCharBoundary*a.config.AdjustmentRate

		// Only adjust if it's a meaningful change
		if abs(newBoundary-currentBoundary) > 0.05 {
			a.decoder.config.InterCharBoundary = newBoundary
			output.TimingAdjusted = true
		}
	}

	// Call the corrected callback
	if a.correctedCallback != nil {
		a.correctedCallback(output)
	}
}

// decodeElements converts elements to decoded text using current decoder state
func (a *AdaptiveDecoder) decodeElements(elements []Element) string {
	var result strings.Builder
	treeIndex := 1

	for _, elem := range elements {
		if elem.IsDah {
			treeIndex = treeIndex*2 + 1
		} else {
			treeIndex = treeIndex * 2
		}

		if treeIndex >= len(MorseTree) {
			treeIndex = 1
			continue
		}

		if elem.IsCharEnd {
			// treeIndex is guaranteed to be in valid range (1 to len(MorseTree)-1)
			// due to the bounds check above
			char := MorseTree[treeIndex]
			if char != 0 {
				result.WriteRune(char)
			}
			treeIndex = 1
		}
	}

	// Handle last character if not ended
	// treeIndex > 1 means we have accumulated elements
	if treeIndex > 1 {
		char := MorseTree[treeIndex]
		if char != 0 {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// GetDecoder returns the underlying decoder
func (a *AdaptiveDecoder) GetDecoder() *Decoder {
	return a.decoder
}

// GetPatternMatchCounts returns the count of pattern matches
func (a *AdaptiveDecoder) GetPatternMatchCounts() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()

	counts := make(map[string]int)
	for k, v := range a.patternMatches {
		counts[k] = v
	}
	return counts
}

// Reset clears the element buffer and match counts
func (a *AdaptiveDecoder) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.elementBuffer = a.elementBuffer[:0]
	a.patternMatches = make(map[string]int)
}

// abs returns absolute value of float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
