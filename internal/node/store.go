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

func (s *Store) NodeByID(id string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, n := range s.nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func (s *Store) Update(id string, update func(Node) Node) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.nodes {
		if s.nodes[i].ID != id {
			continue
		}
		s.nodes[i] = update(s.nodes[i])
		return true
	}
	return false
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
			if bestAvoidedIndex == -1 || better(node, s.nodes[bestAvoidedIndex]) {
				bestAvoidedIndex = i
			}
			continue
		}

		if bestIndex == -1 || better(node, s.nodes[bestIndex]) {
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

func (s *Store) CandidatesByRegion(region, avoidID string, limit int) []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]Node, 0)
	for _, node := range s.nodes {
		if !node.Available || node.Region != region {
			continue
		}
		if avoidID != "" && node.ID == avoidID {
			continue
		}
		candidates = append(candidates, node)
	}
	sortNodes(candidates)
	if limit > 0 && len(candidates) > limit {
		return append([]Node(nil), candidates[:limit]...)
	}
	return append([]Node(nil), candidates...)
}

func better(a, b Node) bool {
	if a.LatencyMS == 0 && b.LatencyMS > 0 {
		return false
	}
	if b.LatencyMS == 0 && a.LatencyMS > 0 {
		return true
	}
	if a.LatencyMS > 0 && b.LatencyMS > 0 && a.LatencyMS != b.LatencyMS {
		return a.LatencyMS < b.LatencyMS
	}
	if a.Speed != b.Speed {
		return a.Speed > b.Speed
	}
	return a.LatencyMS < b.LatencyMS
}

func sortNodes(nodes []Node) {
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && better(nodes[j], nodes[j-1]); j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
}
