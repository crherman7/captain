package graph

import (
	"strings"
	"testing"
)

func mustAddEdge(t *testing.T, g *Graph, from, to string) {
	t.Helper()
	if err := g.AddEdge(from, to); err != nil {
		t.Fatalf("AddEdge(%q, %q): %v", from, to, err)
	}
}

func TestSort_Empty(t *testing.T) {
	g := New()
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestSort_LinearChain(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddNode("c")
	mustAddEdge(t, g, "c", "b")
	mustAddEdge(t, g, "b", "a")

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	posA := indexOf(result, "a")
	posB := indexOf(result, "b")
	posC := indexOf(result, "c")

	if posA > posB || posB > posC {
		t.Errorf("expected a before b before c, got %v", result)
	}
}

func TestSort_DependencyOrder(t *testing.T) {
	g := New()
	g.AddNode("postgres")
	g.AddNode("redis")
	g.AddNode("api")
	g.AddNode("web")
	mustAddEdge(t, g, "api", "postgres")
	mustAddEdge(t, g, "api", "redis")

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	posPostgres := indexOf(result, "postgres")
	posRedis := indexOf(result, "redis")
	posAPI := indexOf(result, "api")

	if posPostgres > posAPI || posRedis > posAPI {
		t.Errorf("expected postgres and redis before api, got %v", result)
	}
}

func TestSort_CycleDetection(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.edges["a"] = []string{"b"}
	g.edges["b"] = []string{"a"}

	_, err := g.Sort()
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestSort_Diamond(t *testing.T) {
	g := New()
	g.AddNode("a")
	g.AddNode("b")
	g.AddNode("c")
	g.AddNode("d")
	mustAddEdge(t, g, "d", "b")
	mustAddEdge(t, g, "d", "c")
	mustAddEdge(t, g, "b", "a")
	mustAddEdge(t, g, "c", "a")

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	posA := indexOf(result, "a")
	posD := indexOf(result, "d")
	if posA > posD {
		t.Errorf("expected a before d, got %v", result)
	}
}

func TestLayers(t *testing.T) {
	g := New()
	g.AddNode("postgres")
	g.AddNode("redis")
	g.AddNode("api")
	g.AddNode("web")
	mustAddEdge(t, g, "api", "postgres")

	layers, err := g.Layers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(layers) < 2 {
		t.Fatalf("expected at least 2 layers, got %d", len(layers))
	}
}

func TestAddEdge_NonexistentNode(t *testing.T) {
	g := New()
	g.AddNode("a")

	if err := g.AddEdge("a", "b"); err == nil {
		t.Error("expected error for nonexistent dependency")
	}
	if err := g.AddEdge("b", "a"); err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func indexOf(s []string, v string) int {
	for i, item := range s {
		if item == v {
			return i
		}
	}
	return -1
}
