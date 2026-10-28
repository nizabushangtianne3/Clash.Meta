// Package ruleindex contains position indexes shared by rule scanners.
package ruleindex

import (
	"slices"
	"strings"
)

// SuffixTrie indexes rule positions by reversed domain labels.
type SuffixTrie struct {
	children  map[string]*SuffixTrie
	positions []int
}

// Add records a suffix at an ascending rule-list position.
func (trie *SuffixTrie) Add(suffix string, position int) {
	node := trie
	labels := strings.Split(suffix, ".")
	for index := len(labels) - 1; index >= 0; index-- {
		if node.children == nil {
			node.children = make(map[string]*SuffixTrie)
		}
		child, ok := node.children[labels[index]]
		if !ok {
			child = &SuffixTrie{}
			node.children[labels[index]] = child
		}
		node = child
	}
	node.positions = append(node.positions, position)
}

// Match returns the earliest position at or after from for any host suffix.
func (trie *SuffixTrie) Match(host string, from int) int {
	hit := -1
	node := trie
	update := func() {
		if position := AtOrAfter(node.positions, from); position >= 0 &&
			(hit < 0 || position < hit) {
			hit = position
		}
	}
	update()
	for end := len(host); end > 0; {
		start := end - 1
		for start >= 0 && host[start] != '.' {
			start--
		}
		child, ok := node.children[host[start+1:end]]
		if !ok {
			break
		}
		node = child
		update()
		end = start
	}
	return hit
}

// AtOrAfter binary-searches an ascending position list.
func AtOrAfter(positions []int, target int) int {
	index, _ := slices.BinarySearch(positions, target)
	if index == len(positions) {
		return -1
	}
	return positions[index]
}
