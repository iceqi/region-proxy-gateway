package node

import "sync"

type Store struct {
	mu    sync.RWMutex
	nodes []Node
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Replace(nodes []Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes = append([]Node(nil), nodes...)
}

func (s *Store) List() []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]Node(nil), s.nodes...)
}

func (s *Store) BestByRegion(region, avoidID string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bestIndex := -1
	bestAvoidedIndex := -1
	for i, node := range s.nodes {
		if !node.Available || node.Region != region {
			continue
		}

		if avoidID != "" && node.ID == avoidID {
			if bestAvoidedIndex == -1 || node.LatencyMS < s.nodes[bestAvoidedIndex].LatencyMS {
				bestAvoidedIndex = i
			}
			continue
		}

		if bestIndex == -1 || node.LatencyMS < s.nodes[bestIndex].LatencyMS {
			bestIndex = i
		}
	}

	if bestIndex != -1 {
		return s.nodes[bestIndex], true
	}
	if bestAvoidedIndex != -1 {
		return s.nodes[bestAvoidedIndex], true
	}
	return Node{}, false
}
