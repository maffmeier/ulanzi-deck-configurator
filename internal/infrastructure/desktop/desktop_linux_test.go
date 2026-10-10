package desktop

import (
	"encoding/json"
	"testing"
)

func TestSwayFocusedNode(t *testing.T) {
	tree := `{"nodes":[{"nodes":[{"app_id":"foot","focused":false},{"app_id":null,"focused":true,"window_properties":{"class":"Code"}}]}],"floating_nodes":[]}`
	var n swayNode
	if err := json.Unmarshal([]byte(tree), &n); err != nil {
		t.Fatal(err)
	}
	f := n.focused()
	if f == nil || f.WindowProperties.Class != "Code" {
		t.Fatalf("got %+v", f)
	}
}
