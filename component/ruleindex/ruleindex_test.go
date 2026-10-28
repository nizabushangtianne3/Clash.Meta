package ruleindex

import "testing"

func TestSuffixTrieReturnsTheEarliestPosition(t *testing.T) {
	trie := &SuffixTrie{}
	trie.Add("example.com", 3)
	trie.Add("target.example.com", 1)
	trie.Add("target.example.com", 5)
	for _, test := range []struct {
		host       string
		from, want int
	}{
		{"www.target.example.com", 0, 1},
		{"www.target.example.com", 2, 3},
		{"www.target.example.com", 4, 5},
		{"www.example.com", 0, 3},
		{"missing.example.net", 0, -1},
	} {
		if got := trie.Match(test.host, test.from); got != test.want {
			t.Fatalf("Match(%q, %d) = %d, want %d", test.host, test.from, got, test.want)
		}
	}
}

func TestAtOrAfterSearchesAscendingPositions(t *testing.T) {
	for _, test := range []struct {
		positions []int
		target    int
		want      int
	}{
		{[]int{1, 4, 7}, 0, 1},
		{[]int{1, 4, 7}, 4, 4},
		{[]int{1, 4, 7}, 5, 7},
		{[]int{1, 4, 7}, 8, -1},
		{nil, 0, -1},
	} {
		if got := AtOrAfter(test.positions, test.target); got != test.want {
			t.Fatalf("AtOrAfter(%v, %d) = %d, want %d", test.positions, test.target, got, test.want)
		}
	}
}
