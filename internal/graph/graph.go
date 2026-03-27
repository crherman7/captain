package graph

import "fmt"

type Graph struct {
	nodes map[string]bool
	edges map[string][]string // from -> [dependencies]
}

func New() *Graph {
	return &Graph{
		nodes: make(map[string]bool),
		edges: make(map[string][]string),
	}
}

func (g *Graph) AddNode(name string) {
	g.nodes[name] = true
}

func (g *Graph) AddEdge(from, to string) error {
	if !g.nodes[to] {
		return fmt.Errorf("dependency %q does not exist", to)
	}
	if !g.nodes[from] {
		return fmt.Errorf("node %q does not exist", from)
	}
	g.edges[from] = append(g.edges[from], to)
	return nil
}

// Sort returns a topological ordering where dependencies come first.
func (g *Graph) Sort() ([]string, error) {
	return g.kahnSort()
}

// Layers returns groups of nodes that can be processed in parallel.
func (g *Graph) Layers() ([][]string, error) {
	sorted, err := g.Sort()
	if err != nil {
		return nil, err
	}

	layerOf := make(map[string]int, len(sorted))
	maxLayer := 0
	for _, name := range sorted {
		layer := 0
		for _, dep := range g.edges[name] {
			if layerOf[dep]+1 > layer {
				layer = layerOf[dep] + 1
			}
		}
		layerOf[name] = layer
		if layer > maxLayer {
			maxLayer = layer
		}
	}

	layers := make([][]string, maxLayer+1)
	for _, name := range sorted {
		l := layerOf[name]
		layers[l] = append(layers[l], name)
	}

	return layers, nil
}

func (g *Graph) kahnSort() ([]string, error) {
	inDegree := make(map[string]int, len(g.nodes))
	for name := range g.nodes {
		inDegree[name] = 0
	}

	reverse := make(map[string][]string)
	for from, deps := range g.edges {
		for _, dep := range deps {
			reverse[dep] = append(reverse[dep], from)
			inDegree[from]++
		}
	}

	var queue []string
	for name := range g.nodes {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	sortStrings(queue)

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, dependent := range reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = insertSorted(queue, dependent)
			}
		}
	}

	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("dependency cycle detected")
	}

	return result, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func insertSorted(s []string, v string) []string {
	i := 0
	for i < len(s) && s[i] < v {
		i++
	}
	s = append(s, "")
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}
