package loginroute

import "testing"

func TestNextSessionPathCoversNodesAndWorkers(t *testing.T) {
	ResetForTest()
	nodes := []string{"login-1", "login-2"}
	seen := map[string]int{}
	for i := 0; i < 16; i++ {
		path := NextSessionPath(nodes, 8)
		seen[path]++
	}
	if len(seen) != 16 {
		t.Fatalf("paths %v", seen)
	}
	for _, node := range nodes {
		for w := 0; w < 8; w++ {
			key := node + ".session." + itoa(w)
			if seen[key] != 1 {
				t.Fatalf("missing %s in %v", key, seen)
			}
		}
	}
	if NextSessionPath(nil, 8) != "" {
		t.Fatal("empty nodes")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
